// Package terminal owns terminal processes, retained output and client subscriptions.
package terminal

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

const MaxSessions = 12

var ErrLimit = errors.New("最多保留 12 个终端，请先关闭不用的会话")

// Manager's zero value is ready for use. Finished sessions remain available
// for output replay until explicitly removed, and still count toward the limit.
type Manager struct {
	mu       sync.Mutex
	sequence int
	sessions map[string]*Session
}

// Start takes ownership of a command and its PTY after a successful start.
// The caller authorizes the workspace and supplies the guest connection.
func (m *Manager) Start(workspace string, cmd *exec.Cmd) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sessions) >= MaxSessions {
		return nil, ErrLimit
	}
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	file, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 100, Rows: 30})
	if err != nil {
		return nil, err
	}
	m.sequence++
	s := &Session{
		id: hex.EncodeToString(id[:]), title: fmt.Sprintf("终端 %d", m.sequence),
		workspace: workspace, cmd: cmd, file: file, clients: map[chan []byte]bool{},
		done: make(chan struct{}),
	}
	if m.sessions == nil {
		m.sessions = map[string]*Session{}
	}
	m.sessions[s.id] = s
	go s.read()
	return s, nil
}

func (m *Manager) List() []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Info{}
	for _, s := range m.sessions {
		out = append(out, s.Info())
	}
	return out
}

func (m *Manager) Get(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

func (m *Manager) Close(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.sessions[id]; s != nil {
		delete(m.sessions, id)
		s.close()
		<-s.done
	}
}

// CloseAll ends processes without removing their summaries or retained output.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		s.close()
	}
	for _, s := range m.sessions {
		<-s.done
	}
}

func (m *Manager) HasActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		if !s.Info().Closed {
			return true
		}
	}
	return false
}
