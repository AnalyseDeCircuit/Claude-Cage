package workbench

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/environment"
	"github.com/sylvinus/agent-vm/internal/managedproxy"
	"github.com/sylvinus/agent-vm/internal/vmname"
)

// OpenService takes exclusive ownership of a profile and starts its monitors.
// managementOrigin is the authenticated endpoint used by boundary checks.
// The host must Close the service before another controller can take ownership.
func OpenService(configPath, managementOrigin string) (*Service, error) {
	profile := filepath.Dir(configPath)
	if err := os.MkdirAll(profile, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(profile, "control.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		return nil, errors.New("此配置已有工作台服务正在运行")
	}
	exe, err := os.Executable()
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	s := &Service{profile: profile, configPath: configPath, exe: exe, origin: managementOrigin, controlLock: lock, preferences: Preferences{Keyboard: "mac"}}
	s.lifetimeContext()
	if err := s.initialize(); err != nil {
		s.lifeCancel()
		_ = lock.Close()
		return nil, err
	}
	s.startMonitors()
	return s, nil
}

func (s *Service) initialize() error {
	identity, err := currentController()
	if err != nil {
		return err
	}
	s.identity = identity
	s.preferences = s.settingsStore().ReadPreferences(s.preferences)
	if err := s.prepareEnvironment(); err != nil {
		return err
	}
	if err := s.restoreNetworkIntent(); err != nil {
		s.networkError = err.Error()
	}
	if s.preferences.Keyboard == "" {
		s.preferences.Keyboard = "mac"
	}
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		if err := writePrivate(s.configPath, defaultConfig()); err != nil {
			return err
		}
	}
	if cfg, err := loadConfig(s.configPath); err != nil || cfg.RequireManagedProxy(s.profile) != nil {
		_ = s.closeProxyGate("尚未配置有效节点，外网保持关闭")
	}
	if s.networkPaused {
		_ = s.closeProxyGate("网络保持暂停，请手动恢复后再联网")
	}
	return nil
}

func (s *Service) lifetimeContextLocked() context.Context {
	if s.lifeContext == nil {
		s.lifeContext, s.lifeCancel = context.WithCancel(context.Background())
		if s.closing {
			s.lifeCancel()
		}
	}
	return s.lifeContext
}

func (s *Service) lifetimeContext() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lifetimeContextLocked()
}

// Activities drain before resources or the profile lock are released. Their
// contexts end on either caller disconnect or control-service shutdown.
func (s *Service) beginActivity(parent context.Context) (context.Context, func(), error) {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return nil, nil, errClosing
	}
	lifetime := s.lifetimeContextLocked()
	s.activities.Add(1)
	s.mu.Unlock()
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(lifetime, cancel)
	return ctx, func() { stop(); cancel(); s.activities.Done() }, nil
}

func (s *Service) startMonitors() {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return
	}
	ctx := s.lifetimeContextLocked()
	s.monitors.Add(2)
	s.mu.Unlock()
	go func() { defer s.monitors.Done(); s.monitor(ctx) }()
	go func() { defer s.monitors.Done(); s.monitorNetwork(ctx) }()
}

func (s *Service) controlContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(s.lifetimeContext(), 5*time.Second)
}

// beginClose is non-blocking so an HTTP handoff can still finish its response
// and release the operation lock before Close drains jobs and monitors.
func (s *Service) beginClose() {
	s.mu.Lock()
	ctx := s.lifetimeContextLocked()
	if ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	s.closing = true
	s.lifeCancel()
	queue := s.jobQueueLocked()
	jobID := queue.Execution().ID()
	if s.checkCancel != nil {
		s.checkCancel()
	}
	if s.monitorCancel != nil {
		s.monitorCancel()
	}
	s.mu.Unlock()
	for _, name := range []string{vmname.Name(s.environmentDir()), "base-building"} {
		if owner := cage.InstanceJobID(filepath.Join(s.profile, "state"), name); owner != "" {
			_ = cage.RequestJobStop(s.profile, owner)
		}
	}
	if jobID != "" {
		_ = cage.RequestJobStop(s.profile, jobID)
	}
	queue.Cancel()
}

// Close exits the controller, not the VM. It closes the network gate and host
// connections, preserving the disk and durable network intent for reconnection.
// Closing a window or subscription must not call this method.
func (s *Service) Close() error {
	s.closeOnce.Do(func() {
		s.beginClose()
		s.jobQueue().Close()
		s.monitors.Wait()
		// A stream handler may be blocked writing input to a full PTY. End
		// existing sessions before waiting for handlers; sweep again below for
		// sessions created by requests admitted before shutdown began.
		s.terminals.CloseAll()
		s.activities.Wait()
		s.op.Lock()
		s.mu.Lock()
		projects, browser := s.projects, s.browserControl
		s.mu.Unlock()
		gateErr := s.closeProxyGate("控制服务正在退出，外网保持关闭")
		s.terminals.CloseAll()
		if projects != nil {
			projects.Close()
		}
		if browser != nil {
			browser.CloseIdleConnections()
		}
		proxyErr := managedproxy.Stop(s.profile)
		s.mu.Lock()
		s.ready = false
		s.credentials = environment.Credentials{}
		s.mu.Unlock()
		s.op.Unlock()
		s.closeErr = errors.Join(gateErr, proxyErr)
		if s.controlPublished {
			s.closeErr = errors.Join(s.closeErr, os.Remove(filepath.Join(s.profile, "control.json")))
		}
		if s.controlLock != nil {
			s.closeErr = errors.Join(s.closeErr, s.controlLock.Close())
		}
	})
	return s.closeErr
}
