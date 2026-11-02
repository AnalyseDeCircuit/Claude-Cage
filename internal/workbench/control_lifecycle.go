package workbench

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"syscall"

	"github.com/sylvinus/agent-vm/internal/managedproxy"
	"github.com/sylvinus/agent-vm/internal/version"
)

const controlProtocol = 1

type ControllerIdentity struct {
	Version  string `json:"version"`
	Build    string `json:"build"`
	Protocol int    `json:"protocol"`
	PID      int    `json:"pid"`
}

func currentController() (ControllerIdentity, error) {
	build, err := version.ExecutableIdentity()
	return ControllerIdentity{Version: version.Version, Build: build, Protocol: controlProtocol, PID: os.Getpid()}, err
}

func (identity ControllerIdentity) matches(other ControllerIdentity) bool {
	return identity.Build != "" && identity.Build == other.Build && identity.Protocol == other.Protocol
}

type launchResponse struct {
	URL        string             `json:"url"`
	Controller ControllerIdentity `json:"controller"`
}

func (s *Server) launcherAuthenticated(w http.ResponseWriter, r *http.Request) bool {
	var input struct {
		Token string `json:"token"`
	}
	if !readJSON(w, r, &input) {
		return false
	}
	if !equalSecret(input.Token, s.token) {
		http.Error(w, "Launcher refused", http.StatusUnauthorized)
		return false
	}
	return true
}

func (s *Server) handoff(w http.ResponseWriter, r *http.Request) {
	if !s.launcherAuthenticated(w, r) {
		return
	}
	if !s.op.TryLock() {
		http.Error(w, "请先保存工作并停止环境，再完成更新。", http.StatusConflict)
		return
	}
	s.mu.Lock()
	busy := s.jobQueueLocked().Snapshot().Running || controllerHasVM(s.profile) || s.terminals.HasActive()
	if busy || s.shutdown == nil {
		s.mu.Unlock()
		s.op.Unlock()
		http.Error(w, "请先保存工作并停止环境，再完成更新。", http.StatusConflict)
		return
	}
	s.closing = true
	s.mu.Unlock()
	if err := managedproxy.Stop(s.profile); err != nil {
		s.mu.Lock()
		s.closing = false
		s.mu.Unlock()
		s.op.Unlock()
		http.Error(w, "代理尚未停止，未继续后台更新。", http.StatusConflict)
		return
	}
	jsonReply(w, map[string]bool{"stopping": true})
	// Keep the lifecycle lock until the listener and current HTTP handlers close.
	// No new job or terminal may start after the idle decision.
	go func() { s.shutdown(); s.op.Unlock() }()
}

func controllerHasVM(profile string) bool {
	records, err := filepath.Glob(filepath.Join(profile, "state", "lima", "*", "cage-running.json"))
	if err != nil {
		return true
	}
	for _, path := range records {
		b, err := os.ReadFile(path)
		if err != nil {
			return true
		}
		var record struct {
			PID int `json:"pid"`
		}
		if json.Unmarshal(b, &record) != nil || record.PID <= 0 {
			return true
		}
		if err := syscall.Kill(record.PID, 0); err == nil || err != syscall.ESRCH {
			return true
		}
	}
	return false
}
