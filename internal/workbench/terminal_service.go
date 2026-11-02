package workbench

import (
	"context"
	"errors"

	"github.com/sylvinus/agent-vm/internal/terminal"
)

var (
	ErrOperationBusy       = errors.New("环境正在执行其他操作")
	ErrTerminalUnavailable = errors.New("环境尚未就绪或正在重启")
	ErrTerminalStart       = errors.New("无法创建终端连接")
)

func (s *Service) Terminals() []terminal.Info { return s.terminals.List() }

// Terminal returns an existing session. Subscriptions and input belong to that
// session; disconnecting a subscriber does not close the process.
func (s *Service) Terminal(id string) *terminal.Session { return s.terminals.Get(id) }

func (s *Service) CreateTerminal(ctx context.Context, workspace string) (terminal.Info, error) {
	ctx, done, err := s.beginActivity(ctx)
	if err != nil {
		return terminal.Info{}, err
	}
	defer done()
	if !s.op.TryLock() {
		return terminal.Info{}, ErrOperationBusy
	}
	defer s.op.Unlock()
	s.mu.Lock()
	running, busy := s.vmRunning(), s.jobQueueLocked().Snapshot().Running
	s.mu.Unlock()
	if !running || busy {
		return terminal.Info{}, ErrTerminalUnavailable
	}
	if len(s.terminals.List()) >= terminal.MaxSessions {
		return terminal.Info{}, terminal.ErrLimit
	}
	workspace, err = s.projectWorkspace(workspace)
	if err != nil {
		return terminal.Info{}, err
	}
	if err := s.projectManager().EnsureShare(ctx, workspace); err != nil {
		return terminal.Info{}, err
	}
	if err := ctx.Err(); err != nil {
		return terminal.Info{}, err
	}
	session, err := s.terminals.Start(workspace, s.environmentRuntime().TerminalCommand(workspace))
	if err != nil {
		return terminal.Info{}, ErrTerminalStart
	}
	return session.Info(), nil
}

func (s *Service) CloseTerminal(id string) error {
	_, done, err := s.beginActivity(context.Background())
	if err != nil {
		return err
	}
	defer done()
	s.terminals.Close(id)
	return nil
}
