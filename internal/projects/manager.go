// Package projects owns directory grants and their live guest shares.
package projects

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Guest runs commands through the host's authenticated guest connection.
type Guest interface {
	Run(context.Context, string) ([]byte, error)
	Command(context.Context, string) *exec.Cmd
}

// Manager keeps directory grants separate from the UI's selected project.
type Manager struct {
	profile, environment string
	guest                Guest
	op                   sync.Mutex
	shares               map[string]*share
	statusMu             sync.Mutex
	restoreError         string
	transports           sync.WaitGroup
}

func New(profile, environment string, guest Guest) *Manager {
	return &Manager{profile: profile, environment: environment, guest: guest}
}

func (m *Manager) EnsureShare(ctx context.Context, path string) error {
	m.op.Lock()
	defer m.op.Unlock()
	return m.ensureShare(ctx, path)
}

func (m *Manager) Remember(path string) error {
	m.op.Lock()
	defer m.op.Unlock()
	return m.remember(path)
}

// Restore reconnects saved directories, including the selected directory from
// profiles created before projects.json existed. One failure does not stop others.
func (m *Manager) Restore(ctx context.Context, current string) {
	m.op.Lock()
	defer m.op.Unlock()
	var saved sharedProjects
	if b, err := os.ReadFile(filepath.Join(m.profile, "projects.json")); err == nil {
		_ = json.Unmarshal(b, &saved)
	}
	if current != "" && current != m.environment {
		found := false
		for _, p := range saved.Paths {
			found = found || p == current
		}
		if !found {
			saved.Paths = append(saved.Paths, current)
		}
	}
	failed := false
	for _, path := range saved.Paths {
		if err := m.ensureShare(ctx, path); err != nil {
			failed = true
		} else if err := m.remember(path); err != nil {
			failed = true
		}
	}
	m.statusMu.Lock()
	m.restoreError = ""
	if failed {
		m.restoreError = "部分项目文件夹未能恢复连接，请重新打开对应项目。环境和浏览器可继续使用。"
	}
	m.statusMu.Unlock()
}

func (m *Manager) RestoreError() string {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	return m.restoreError
}

func (m *Manager) Close() {
	m.op.Lock()
	defer m.op.Unlock()
	for _, share := range m.shares {
		share.close()
	}
	m.transports.Wait()
	m.shares = nil
}
