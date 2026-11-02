package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sylvinus/agent-vm/internal/browser"
	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/projects"
	"github.com/sylvinus/agent-vm/internal/vmname"
)

func TestTerminalAndEditorCommandsWithoutHTTP(t *testing.T) {
	s := testService(t)
	defer s.Close()
	ctx := context.Background()
	if _, err := s.CreateTerminal(ctx, ""); !errors.Is(err, ErrTerminalUnavailable) {
		t.Fatal("terminal creation bypassed the environment check", err)
	}
	if _, err := s.BrowserTabs(ctx); !errors.Is(err, ErrBrowserUnavailable) {
		t.Fatal("browser command bypassed readiness", err)
	}
	s.ready = true
	s.preferences.BrowserHomeURL = "https://example.com/home"
	s.credentials.Token = "service-guest-token"
	project, _ := filepath.EvalSymlinks(t.TempDir())
	other, _ := filepath.EvalSymlinks(t.TempDir())
	s.preferences.Workspace = other
	if err := s.rememberSharedProject(project); err != nil {
		t.Fatal(err)
	}
	s.projects = projects.New(s.profile, s.environmentDir(), projectGuestStub{})
	marker := filepath.Join(s.profile, "state", "lima", vmname.Name(s.environmentDir()), "cage-running.json")
	if err := writePrivate(marker, map[string]int{"pid": os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	// Only the browser guest endpoint is HTTP; the workbench has no Server.
	guest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer service-guest-token" {
			t.Error("service lost guest authentication")
		}
		var input browser.Input
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if r.URL.Path == "/new" && input.URL != "https://example.com/home" {
			t.Error("service did not apply the default website")
		}
		if strings.HasPrefix(r.URL.Path, "/editor") && input.Workspace != project {
			t.Error("editor did not use its authorized directory")
		}
		_, _ = w.Write([]byte(`{"id":"editor"}`))
	}))
	defer guest.Close()
	s.browserControl = browser.New(guest.URL)
	for _, action := range []string{"new", "editor", "editor-restart"} {
		if _, err := s.BrowserCommand(ctx, action, browser.Input{Workspace: project}); err != nil {
			t.Fatal(action, err)
		}
	}
	info, err := s.CreateTerminal(ctx, project)
	if err != nil || info.Workspace != project || s.Terminal(info.ID) == nil || len(s.Terminals()) != 1 {
		t.Fatal("service did not create the authorized terminal", info, err)
	}
	if s.workspace() != other {
		t.Fatal("opening the editor or terminal changed the selected project")
	}
	for _, path := range []string{t.TempDir(), s.profile} {
		if _, err := s.CreateTerminal(ctx, path); err == nil {
			t.Fatal("direct terminal creation accepted an unauthorized directory")
		}
		if _, err := s.BrowserCommand(ctx, "editor", browser.Input{Workspace: path}); err == nil {
			t.Fatal("direct editor operation accepted an unauthorized directory")
		}
	}
	s.op.Lock()
	_, terminalErr := s.CreateTerminal(ctx, project)
	_, editorErr := s.BrowserCommand(ctx, "editor-restart", browser.Input{Workspace: project})
	s.op.Unlock()
	if !errors.Is(terminalErr, ErrOperationBusy) || !errors.Is(editorErr, ErrOperationBusy) {
		t.Fatal("direct commands bypassed operation exclusion", terminalErr, editorErr)
	}
	if err := s.CloseTerminal(info.ID); err != nil || s.Terminal(info.ID) != nil {
		t.Fatal("service did not close its terminal", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTerminal(ctx, project); !errors.Is(err, errClosing) {
		t.Fatal("closed service accepted a terminal", err)
	}
	if _, err := s.BrowserCommand(ctx, "new", browser.Input{}); !errors.Is(err, errClosing) {
		t.Fatal("closed service accepted a browser command", err)
	}
}

func TestDirectBrowserSubscriptionEndsWithService(t *testing.T) {
	s := testService(t)
	s.ready = true
	ended := make(chan struct{})
	guest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "data: {\"tabs\":[]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(ended)
	}))
	defer guest.Close()
	s.browserControl = browser.New(guest.URL)
	defer s.Close()
	stream, err := s.SubscribeBrowser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ended:
	case <-time.After(3 * time.Second):
		t.Fatal("service retained a direct browser subscription")
	}
}

func TestServiceSavesSettingsAndStopsResourcesWithoutHTTP(t *testing.T) {
	service := testService(t)
	service.ready = true
	terminal, err := service.terminals.Start(t.TempDir(), exec.Command("cat"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.terminals.CloseAll)
	output := terminal.Subscribe()
	defer output.Close()
	input, err := service.ReadSettings()
	if err != nil {
		t.Fatal(err)
	}
	input.Keyboard = "native"
	queued, err := service.SaveSettings(input)
	if err != nil || queued.Kind != "settings" {
		t.Fatal("service did not accept settings directly", queued, err)
	}
	if job := waitJobFinished(t, service); job.ID != queued.ID || job.Error != "" {
		t.Fatal("service settings command failed", job)
	}
	current, err := service.ReadSettings()
	if err != nil || current != input || service.settingsStore().ReadPreferences(Preferences{}).Keyboard != "native" {
		t.Fatal("settings were not applied and persisted", err)
	}
	status, err := service.Snapshot()
	if err != nil || !status.Ready || status.Keyboard != "native" || terminal.Info().Closed {
		t.Fatal("live preference save interrupted service resources", err)
	}
	stop, err := service.Stop()
	if err != nil || stop.Kind != "stop" {
		t.Fatal("service did not accept stop directly", err)
	}
	if job := waitJobFinished(t, service); job.ID != stop.ID || job.Error != "" {
		t.Fatal("service stop failed", job)
	}
	select {
	case _, open := <-output.Output:
		if open {
			t.Fatal("stopped terminal remained open")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("service did not reap its terminal process")
	}
	status, err = service.Snapshot()
	if err != nil || status.Ready || status.VMRunning || !terminal.Info().Closed || !service.networkPaused {
		t.Fatal("service stop retained active resources or resumed networking", err)
	}
}

func TestServiceStopSupersedesQueuedStartBeforeVMWork(t *testing.T) {
	service := testService(t)
	service.op.Lock()
	locked := true
	defer func() {
		if locked {
			service.op.Unlock()
		}
	}()
	start, err := service.Start()
	if err != nil || start.Kind != "start" {
		t.Fatal("service did not accept start directly", err)
	}
	stop, err := service.Stop()
	if err != nil || stop.ID == start.ID {
		t.Fatal("stop did not supersede the queued start", err)
	}
	again, err := service.Stop()
	if err != nil || again.ID != stop.ID {
		t.Fatal("repeated stop replaced the pending stop", err)
	}
	service.op.Unlock()
	locked = false
	if result := waitJobFinished(t, service); result.ID != stop.ID || result.Error != "" {
		t.Fatal("queued service stop failed", result)
	}
	if _, err := os.Stat(filepath.Join(service.profile, "environment.json")); !os.IsNotExist(err) {
		t.Fatal("superseded start reached environment preparation", err)
	}
	if !service.networkPaused {
		t.Fatal("stop did not retain its network pause")
	}
}

func TestServiceValidatesSettingsAndRestoreBeforeAdmission(t *testing.T) {
	service := testService(t)
	before, err := os.ReadFile(service.configPath)
	if err != nil {
		t.Fatal(err)
	}
	input, err := service.ReadSettings()
	if err != nil {
		t.Fatal(err)
	}
	input.Timezone = "Invented/Invalid"
	if _, err := service.SaveSettings(input); err == nil {
		t.Fatal("direct service command bypassed settings validation")
	}
	if _, err := service.RestoreSettings(); err == nil || err.Error() != "没有可恢复的配置" {
		t.Fatal("direct restore accepted a missing backup", err)
	}
	after, err := os.ReadFile(service.configPath)
	if err != nil || string(before) != string(after) || service.jobQueue().Snapshot() != (Job{}) {
		t.Fatal("rejected command changed configuration or published a job", err)
	}
}

func TestSnapshotKeepsWireFieldsAndDoesNotExposeMutableState(t *testing.T) {
	service := testService(t)
	service.location = &Location{IP: "192.0.2.1", Country: "test country"}
	report := &cage.SafetyReport{ID: "report", Checks: []cage.SafetyCheck{{ID: "check", Title: "original", Evidence: "original evidence"}}}
	service.safety.Report = report
	service.safety.Gate = &cage.GateStatus{Mode: cage.GateClosed, Report: report}
	service.safety.pauseVersion = 123
	service.credentials.Token = "private-guest-token"
	service.credentials.VNCPassword = "private-vnc-password"
	snapshot, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	directory := service.environmentDir()
	// This is the previous /api/status contract, including null and zero fields.
	want := map[string]any{
		"projects": []string{directory}, "ready": false, "vm_running": false,
		"workspace": "", "environment_workspace": false, "environment_directory": directory,
		"project_error": "", "keyboard": "mac", "auto_timezone": false, "job": Job{},
		"location": service.location, "network_error": "", "can_restore": false,
		"safety": service.safety, "environment_update_required": false, "environment_update_error": "",
	}
	decode := func(value any) map[string]any {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "private-guest-token") || strings.Contains(string(data), "private-vnc-password") || strings.Contains(string(data), "pauseVersion") {
			t.Fatal("private controls appeared in status")
		}
		var result map[string]any
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if !reflect.DeepEqual(decode(snapshot), decode(want)) {
		t.Fatal("typed snapshot changed the status JSON contract")
	}
	snapshot.Projects[0] = "/changed"
	snapshot.Location.Country = "changed"
	snapshot.Safety.Gate.Mode = cage.GateOpen
	snapshot.Safety.Gate.Report.Checks[0].Evidence = "changed"
	snapshot.Safety.Report.Checks[0].Title = "changed"
	if service.location.Country != "test country" || service.safety.Gate.Mode != cage.GateClosed || report.Checks[0].Title != "original" || report.Checks[0].Evidence != "original evidence" {
		t.Fatal("snapshot mutation changed live service state")
	}
	next, err := service.Snapshot()
	if err != nil || next.Projects[0] != directory {
		t.Fatal("snapshot mutation changed project state", err)
	}
}
