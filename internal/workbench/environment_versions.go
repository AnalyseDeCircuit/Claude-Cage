package workbench

import (
	"context"
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/environment"
	"github.com/sylvinus/agent-vm/internal/managedproxy"
)

// Version commands run only for this explicit request, never in status polling.
func (s *Server) environmentVersions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	snapshot := s.environmentRuntime().Versions(ctx)
	var proxy struct {
		Version string `json:"version"`
	}
	_ = managedproxy.API(ctx, s.profile, "GET", "/version", nil, &proxy)
	requiredProxy := ""
	if core, err := managedproxy.CorePath(); err == nil {
		probeCtx, done := context.WithTimeout(ctx, 2*time.Second)
		b, e := exec.CommandContext(probeCtx, core, "-v").Output()
		done()
		if e == nil {
			fields := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])
			if len(fields) >= 3 && fields[0] == "Mihomo" {
				requiredProxy = fields[2]
			}
		}
	}
	snapshot.Tools["proxy"] = environment.ToolVersion{Current: proxy.Version, Required: requiredProxy}
	s.mu.Lock()
	busy := s.jobQueueLocked().Snapshot().Running
	allowed := s.toolsNetworkAllowedLocked()
	identity := s.identity
	s.mu.Unlock()
	jsonReply(w, map[string]any{"vm_running": snapshot.Running, "busy": busy, "components": snapshot.Components, "controller": identity, "tools": snapshot.Tools, "tools_update_allowed": allowed && snapshot.Components.Status == "current" && !busy, "error": snapshot.Error})
}

func (s *Server) updateEnvironment(w http.ResponseWriter, r *http.Request) {
	job, err := s.UpdateComponents()
	jobReply(w, job, err)
}
func (s *Server) updateEnvironmentTools(w http.ResponseWriter, r *http.Request) {
	job, err := s.UpdateTools()
	jobReply(w, job, err)
}

func (s *Service) toolsNetworkAllowedLocked() bool {
	gate := s.safety.Gate
	return s.ready && !s.networkPaused && gate != nil && gate.Mode == cage.GateOpen && gate.Until.After(time.Now())
}
func (s *Service) upgradeEnvironmentTools() error {
	if !s.vmRunning() {
		return errors.New("请先启动 Cage 环境。")
	}
	s.mu.Lock()
	allowed := s.toolsNetworkAllowedLocked()
	manifest := s.guestManifest
	s.mu.Unlock()
	if !allowed {
		return errors.New("工具升级需要已放行的网络。请先恢复外网并通过检查，再重新点击更新。")
	}
	if manifest == nil || !manifest.Current() {
		return environment.ErrUpdateRequired
	}
	client, err := s.gateClient()
	if err != nil {
		return err
	}
	state, err := client.Status(s.operationContext())
	if err != nil || state.Mode != cage.GateOpen || !state.Until.After(time.Now()) {
		return errors.New("网络放行已失效，请先重新检查再更新工具。")
	}
	monitorContext, stopMonitor := context.WithCancel(s.operationContext())
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-monitorContext.Done():
				return
			case <-ticker.C:
				s.monitorSafety()
			}
		}
	}()
	finishMonitor := func() {
		stopMonitor()
		s.mu.Lock()
		if s.monitorCancel != nil {
			s.monitorCancel()
		}
		s.mu.Unlock()
		<-monitorDone
	}
	s.setMessage("正在更新 Chromium、编辑器和开发工具，工作台终端与磁盘保持运行…")
	err = s.runCage("run", "bash", "-c", cage.ToolsUpdateScript())
	finishMonitor()
	if err != nil {
		return err
	}
	return s.synchronizeEnvironment()
}
