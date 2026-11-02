package workbench

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
)

func TestComponentUpdateFailureClosesNetworkAndKeepsSessions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		paused bool
	}{{"online", false}, {"paused", true}} {
		t.Run(tc.name, func(t *testing.T) {
			s, gate := grantedToolUpdateServer(t)
			holdTestJob(t, s, "environment-update")
			if tc.paused {
				if err := s.pauseNetworkNow("manual"); err != nil {
					t.Fatal(err)
				}
			}
			// The command fails before reconnection, without starting a VM.
			// Pin the synthetic VM identity before resolving the new directory's symlinks.
			directory := s.environmentDir()
			if err := writePrivate(filepath.Join(s.profile, "environment.json"), map[string]string{"directory": directory}); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(directory) })
			s.exe = filepath.Join(t.TempDir(), "cage-test")
			if err := os.WriteFile(s.exe, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\"\nexit 1\n"), 0700); err != nil {
				t.Fatal(err)
			}
			terminal := startTestTerminal(t, s)
			err := s.synchronizeEnvironment()
			job := s.jobQueue().Snapshot()
			if err == nil || !strings.Contains(err.Error(), "up 未完成") || job.Log != "up\n" {
				t.Fatalf("update failure was not reported: %v, %q", err, job.Log)
			}
			if s.ready || !s.guestUpdateRequired || s.guestUpdateError == "" {
				t.Fatal("failed component update was published as ready")
			}
			if gate.Status().Mode != cage.GateClosed || s.networkPaused != tc.paused {
				t.Fatal("component update changed manual pause or left network open")
			}
			if s.terminals.Get(terminal.Info().ID) != terminal || terminal.Info().Closed {
				t.Fatal("component update interrupted an existing terminal")
			}
		})
	}
}

func grantedToolUpdateServer(t *testing.T) (*Server, *cage.EgressGate) {
	t.Helper()
	s, gate := interfaceMonitorServer(t)
	state := gate.Status()
	report := cage.SafetyReport{Policy: cage.SafetyPolicyVersion, ID: "tool-update-test", RunID: state.RunID, CheckID: state.CheckID, Configuration: state.Configuration, CreatedAt: time.Now()}
	for _, id := range []string{"egress_policy", "proxy_transport", "dns_transport", "guest_settings", "browser_settings", "filesystem_boundary", "management_boundary", "fail_closed", "udp_policy", "ipv6_policy", "exit_consistency"} {
		report.Checks = append(report.Checks, cage.SafetyCheck{ID: id, Title: id, Critical: true, State: cage.CheckPassed, Evidence: "synthetic isolated fixture", CheckedAt: report.CreatedAt})
	}
	if err := gate.Grant(report); err != nil {
		t.Fatal(err)
	}
	s.ready = true
	required := cage.RequiredGuestManifest()
	s.guestManifest = &required
	holdTestJob(t, s, "environment-tools")
	s.publishGate(gate.Status())
	return s, gate
}

func TestToolUpdateLeaseHonorsPauseStopAndConfigurationAtRenewal(t *testing.T) {
	for _, condition := range []string{"allowed", "paused", "stop", "configuration-changed"} {
		t.Run(condition, func(t *testing.T) {
			s, gate := grantedToolUpdateServer(t)
			client, err := s.gateClient()
			if err != nil {
				t.Fatal(err)
			}
			before := gate.Status()
			switch condition {
			case "paused":
				if err := s.pauseNetworkNow("test user pause"); err != nil {
					t.Fatal(err)
				}
			case "stop":
				holdTestJob(t, s, "stop")
			case "configuration-changed":
				cfg, err := loadConfig(s.configPath)
				if err != nil {
					t.Fatal(err)
				}
				cfg.Timezone = "Etc/UTC"
				if cfg.Fingerprint() == before.Configuration {
					cfg.Timezone = "Asia/Tokyo"
				}
				if err := writePrivate(s.configPath, cfg); err != nil {
					t.Fatal(err)
				}
			}
			s.finishSafetyMonitor(context.Background(), client, before, "192.0.2.1", "")
			after := gate.Status()
			switch condition {
			case "allowed":
				if after.Mode != cage.GateOpen || !after.Until.After(before.Until) {
					t.Fatal("tool update lease was not renewed")
				}
			case "paused", "configuration-changed":
				if after.Mode != cage.GateClosed {
					t.Fatal("denied network was renewed")
				}
			case "stop":
				if after.Until != before.Until {
					t.Fatal("stopping job allowed a lease renewal")
				}
			}
		})
	}
}

func TestToolUpdateDoesNotRestorePausedOrExpiredNetworking(t *testing.T) {
	for _, paused := range []bool{true, false} {
		s, gate := grantedToolUpdateServer(t)
		if paused {
			if err := s.pauseNetworkNow("manual"); err != nil {
				t.Fatal(err)
			}
		} else {
			gate.Close("expired")
		}
		err := s.upgradeEnvironmentTools()
		if err == nil || !strings.Contains(err.Error(), "网络") {
			t.Fatalf("closed gate accepted: %v", err)
		}
		if gate.Status().Mode != cage.GateClosed {
			t.Fatal("tool update restored closed network")
		}
	}
}

func TestEnvironmentVersionRoutesRequireHostSession(t *testing.T) {
	s := testServer(t)
	for _, path := range []string{"/api/environment/versions", "/api/environment/update", "/api/environment/tools"} {
		method := "POST"
		if strings.HasSuffix(path, "versions") {
			method = "GET"
		}
		if w := request(s, method, path, s.origin, "{}", false); w.Code != 401 {
			t.Fatalf("untrusted version action accepted: %d", w.Code)
		}
	}
	w := request(s, "GET", "/api/environment/versions", s.origin, "", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"stopped"`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
