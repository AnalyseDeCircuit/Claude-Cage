package workbench

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/environment"
	"github.com/sylvinus/agent-vm/internal/managedproxy"
	"github.com/sylvinus/agent-vm/internal/projects"
)

// Runtime operations are independent of HTTP and UI state. The workbench keeps
// the job lock, network authorization and session coordination in their existing order.
func (s *Service) environmentRuntime() *environment.Runtime {
	return environment.New(s.profile, s.configPath, s.exe)
}
func (s *Service) environmentDir() string { return s.environmentRuntime().Directory() }
func (s *Service) vmRunning() bool        { return s.environmentRuntime().Running() }
func (s *Service) sshCommand(ctx context.Context, args ...string) *exec.Cmd {
	return s.environmentRuntime().SSHCommand(ctx, args...)
}
func (s *Service) guest(ctx context.Context, script string) ([]byte, error) {
	return s.environmentRuntime().Guest(ctx, script)
}
func (s *Service) environmentOperation() environment.Operation {
	execution := s.jobQueue().Execution()
	return environment.Operation{JobID: execution.ID(), Output: execution, Progress: execution.SetMessage}
}
func (s *Service) runCage(args ...string) error {
	return s.environmentRuntime().Run(s.operationContext(), s.environmentOperation(), args...)
}

func (s *Service) prepareEnvironment() error {
	path, err := s.environmentRuntime().Prepare(s.workspace())
	if err != nil {
		return err
	}
	s.mu.Lock()
	p := s.preferences
	s.mu.Unlock()
	if p.Workspace == "" {
		p.Workspace = path
	}
	if err := s.settingsStore().SavePreferences(p); err != nil {
		return err
	}
	s.mu.Lock()
	s.preferences = p
	s.mu.Unlock()
	return nil
}

func (s *Service) attach() error {
	ctx, cancel := s.controlContext()
	defer cancel()
	connection, err := s.environmentRuntime().Connection(ctx)
	if connection == nil {
		return err
	}
	credentials, manifest := connection.Credentials, connection.Manifest
	s.mu.Lock()
	s.guestManifest = manifest
	s.guestUpdateRequired = errors.Is(err, environment.ErrUpdateRequired)
	s.guestUpdateError = ""
	if err != nil {
		s.ready = false
		s.guestUpdateError = err.Error()
	}
	s.mu.Unlock()
	if err != nil {
		if errors.Is(err, environment.ErrUpdateRequired) {
			_ = s.closeProxyGate("环境组件需要更新，外网保持关闭")
		}
		return err
	}
	if err = s.browserClient().Health(ctx, credentials.Token); err != nil {
		return err
	}
	s.mu.Lock()
	home := s.preferences.HomeURL()
	s.mu.Unlock()
	if err = s.browserClient().SetHome(ctx, credentials.Token, home); err != nil {
		return err
	}
	s.mu.Lock()
	s.credentials = credentials
	s.ready = true
	s.mu.Unlock()
	s.restoreProjectShares()
	return nil
}
func (s *Service) monitor(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if s.op.TryLock() {
			if !s.busy() {
				s.mu.Lock()
				ready := s.ready
				token := s.credentials.Token
				s.mu.Unlock()
				if !ready {
					_ = s.attach()
				} else {
					ctx, cancel := s.controlContext()
					err := s.browserClient().Health(ctx, token)
					cancel()
					if err != nil {
						s.mu.Lock()
						s.ready = false
						s.mu.Unlock()
					}
				}
			}
			s.op.Unlock()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) startEnvironment(syncTimezone bool) error {
	// Check before proxy activation or the first image download. The UI cannot
	// bypass this gate by calling /start or applying settings directly.
	if err := requireHostGit(s.operationContext()); err != nil {
		return err
	}
	if _, err := projects.ValidateWorkspace(s.environmentDir(), s.profile); err != nil {
		return err
	}
	cfg, err := loadConfig(s.configPath)
	if err != nil {
		return err
	}
	if err = cfg.RequireManagedProxy(s.profile); err != nil {
		_ = s.closeProxyGate(err.Error())
		return err
	}
	{
		p, e := managedproxy.Load(s.profile)
		if e != nil {
			return e
		}
		if p.Selected == "" || p.Endpoint() != cfg.ProxyURL || p.Revision != cfg.ProxyRevision {
			return errors.New("内置代理配置未应用，请重新选择节点")
		}
		name, e := physicalProxyInterface(s.operationContext())
		if e != nil {
			_ = s.closeProxyGate(e.Error())
			return e
		}
		bound, _ := managedproxy.BoundInterface(filepath.Join(managedproxy.Dir(s.profile), "config.yaml"))
		if bound != name || !managedproxy.Running(s.operationContext(), s.profile, p) {
			// Every core start changes the report generation, including recovery.
			if e = s.activateProxy(p, cfg); e != nil {
				return e
			}
			cfg, e = loadConfig(s.configPath)
			if e != nil {
				return e
			}
		}
	}

	attachErr := s.attach()
	if errors.Is(attachErr, environment.ErrUpdateRequired) {
		return s.synchronizeEnvironment()
	}
	if attachErr == nil {
		client, gateErr := s.gateClient()
		if gateErr == nil {
			state, e := client.Status(context.Background())
			if e == nil {
				if state.Mode == cage.GateOpen && state.Report != nil && state.Report.Authorizes(state.RunID, cfg.Fingerprint(), time.Now()) == nil {
					s.publishGate(state)
					return nil
				}
				return s.runSafetyCheck()
			}
		}
		s.setMessage("正在重启环境以加载 VM 外网络门槛…")
		if err := s.stopEnvironment(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	preferences := s.preferences
	auto := preferences.AutoTimezone || preferences.AutoLanguage
	s.mu.Unlock()
	s.setMessage("正在验证代理出口…")
	loc, probeErr := probeLocation(s.operationContext(), cfg)
	if s.operationContext().Err() != nil {
		return s.operationContext().Err()
	}
	s.mu.Lock()
	if probeErr != nil {
		s.networkError = probeErr.Error()
	} else {
		s.location = &loc
		s.networkError = ""
	}
	s.mu.Unlock()
	if cfg.ExitPolicy.NeedsBinding() && loc.IP != "" && !s.vmRunning() {
		bound := cfg.ExitPolicy.BindMissing(loc.exitLocation())
		if bound != cfg.ExitPolicy {
			cfg.ExitPolicy = bound
			if err = writePrivate(s.configPath, cfg); err != nil {
				return err
			}
		}
	}
	if auto && syncTimezone && !s.vmRunning() {
		if probeErr != nil {
			if _, e := os.Stat(filepath.Join(s.profile, "state", ".agent-vm-base-version")); e != nil {
				return probeErr
			}
		}
		if probeErr == nil {
			observed := loc.exitLocation()
			within, _ := cfg.ExitPolicy.Evaluate(loc.IP, &observed)
			updated := cfg
			if within == cage.CheckPassed {
				updated = autoRegionSettings(cfg, preferences, loc)
			}
			if updated != cfg {
				// Changing a live hostagent's profile is forbidden. Stop before replacing it.
				s.setMessage("正在应用出口所在地的时区与语言建议…")
				if err = s.stopEnvironment(); err != nil {
					return err
				}
				cfg = updated
				if err = writePrivate(s.configPath, cfg); err != nil {
					return err
				}
			}
		}
	} else if probeErr != nil {
		s.setMessage("正在检查代理连通性…")
		if _, e := os.Stat(filepath.Join(s.profile, "state", ".agent-vm-base-version")); e != nil {
			if err = s.runCage("check"); err != nil {
				return err
			}
		}
	}
	if err = s.environmentRuntime().Start(s.operationContext(), s.environmentOperation()); err != nil {
		return err
	}
	for i := 0; i < 8; i++ {
		if s.operationContext().Err() != nil {
			return s.operationContext().Err()
		}
		if err = s.attach(); err == nil {
			return s.runSafetyCheck()
		}
		time.Sleep(time.Second)
	}
	return errors.New("环境已启动，但浏览器连接尚未就绪。请重试连接或查看操作详情。")
}
func (s *Service) stopEnvironment() (returned error) {
	s.setMessage("正在停止 Cage 环境…")
	s.mu.Lock()
	explicitStop := s.jobQueueLocked().Snapshot().Kind == "stop"
	s.mu.Unlock()
	defer func() {
		if explicitStop && returned == nil {
			returned = managedproxy.Stop(s.profile)
		}
	}()
	if explicitStop && cage.InstanceJobID(filepath.Join(s.profile, "state"), "base-building") != "" {
		if err := s.runCage("stop", "base-building"); err != nil {
			return err
		}
	}
	if client, err := s.gateClient(); err == nil {
		if state, e := client.Close(context.Background(), "环境正在停止"); e == nil {
			s.publishGate(state)
		}
	}
	s.mu.Lock()
	s.ready = false
	s.credentials = environment.Credentials{}
	s.guestManifest = nil
	s.guestUpdateRequired = false
	s.guestUpdateError = ""
	s.safety.Error = ""
	s.mu.Unlock()
	s.terminals.CloseAll()
	s.closeProjectShares()
	if s.workspace() == "" {
		return nil
	}
	return s.environmentRuntime().Stop(s.operationContext(), s.environmentOperation())
}

func (s *Service) synchronizeEnvironment() error {
	if !s.vmRunning() {
		return errors.New("请先启动 Cage 环境。")
	}
	if err := s.closeProxyGate("正在更新环境组件，外网暂时关闭"); err != nil {
		return err
	}
	s.mu.Lock()
	s.ready = false
	s.guestUpdateRequired = true
	s.guestUpdateError = "环境组件更新尚未完成，请等待结果；失败时可以重试。"
	s.mu.Unlock()
	s.setMessage("正在重启浏览器与编辑器组件，工作台终端保持运行…")
	if err := s.environmentRuntime().ApplyComponents(s.operationContext(), s.environmentOperation()); err != nil {
		return err
	}
	if err := s.attach(); err != nil {
		return err
	}
	s.mu.Lock()
	paused := s.networkPaused
	s.mu.Unlock()
	if paused {
		return nil
	}
	return s.runSafetyCheck()
}
