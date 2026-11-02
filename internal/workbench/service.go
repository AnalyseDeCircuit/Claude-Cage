package workbench

import (
	"context"
	"os"
	"sync"

	"github.com/sylvinus/agent-vm/internal/browser"
	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/environment"
	"github.com/sylvinus/agent-vm/internal/jobs"
	"github.com/sylvinus/agent-vm/internal/projects"
	"github.com/sylvinus/agent-vm/internal/settings"
	"github.com/sylvinus/agent-vm/internal/terminal"
)

// Service owns the profile's running resources and business state. Its commands
// are trusted in-process operations; transport authentication belongs to Server.
type Service struct {
	profile, configPath, exe string
	// The management origin is runtime topology needed by boundary probes.
	origin   string
	identity ControllerIdentity

	mu                  sync.Mutex
	preferences         Preferences
	ready               bool
	closing             bool
	guestManifest       *cage.GuestManifest
	guestUpdateRequired bool
	guestUpdateError    string
	credentials         environment.Credentials

	op             sync.Mutex
	jobs           *jobs.Queue
	browserControl *browser.Client
	terminals      terminal.Manager
	projects       *projects.Manager
	favoritesMu    sync.Mutex

	safetyOp           sync.Mutex
	safety             safetyState
	location           *Location
	locations          map[string]Location
	networkError       string
	monitorCancel      context.CancelFunc
	networkPaused      bool
	proxyNetworkLost   bool
	checkCancel        context.CancelFunc
	activePauseVersion uint64

	lifeContext      context.Context
	lifeCancel       context.CancelFunc
	monitors         sync.WaitGroup
	activities       sync.WaitGroup
	closeOnce        sync.Once
	closeErr         error
	controlLock      *os.File
	controlPublished bool
}

func (s *Service) Start() (Job, error) {
	return s.submit("start", func() error {
		if err := s.prepareEnvironment(); err != nil {
			return err
		}
		return s.startEnvironment(true)
	})
}

func (s *Service) ReadSettings() (Settings, error) {
	c, err := loadConfig(s.configPath)
	if err != nil {
		return Settings{}, err
	}
	s.mu.Lock()
	p := s.preferences
	s.mu.Unlock()
	return settings.View(c, p), nil
}

func (s *Service) proposeSettings(input Settings) (savedProfile, error) {
	old, err := loadConfig(s.configPath)
	if err != nil {
		return savedProfile{}, err
	}
	c, p, err := settings.Propose(input, old, s.profile)
	if err != nil {
		return savedProfile{}, err
	}
	return savedProfile{Config: c, Preferences: p}, nil
}

func (s *Service) SaveSettings(input Settings) (Job, error) {
	v, err := s.proposeSettings(input)
	if err != nil {
		return Job{}, err
	}
	kind := "settings"
	old, err := loadConfig(s.configPath)
	s.mu.Lock()
	previous := s.preferences
	s.mu.Unlock()
	if err == nil && !v.RequiresRestart(savedProfile{Config: old, Preferences: previous}) && previous.Workspace != v.Preferences.Workspace {
		kind = "project"
	}
	return s.submit(kind, func() error { return s.apply(v, true) })
}

func (s *Service) ProbeSettings(ctx context.Context, input Settings) (Location, error) {
	ctx, done, err := s.beginActivity(ctx)
	if err != nil {
		return Location{}, err
	}
	defer done()
	v, err := s.proposeSettings(input)
	if err != nil {
		return Location{}, err
	}
	if err := v.Config.RequireManagedProxy(s.profile); err != nil {
		return Location{}, err
	}
	loc, err := probeLocation(ctx, v.Config)
	if err != nil {
		return Location{}, err
	}
	current, _ := loadConfig(s.configPath)
	if current.ProxyURL == v.Config.ProxyURL && current.DNSURL == v.Config.DNSURL {
		s.mu.Lock()
		s.location = &loc
		s.networkError = ""
		s.mu.Unlock()
	}
	return loc, nil
}

func (s *Service) RestoreSettings() (Job, error) {
	v, err := s.settingsStore().Previous()
	if err != nil {
		return Job{}, err
	}
	return s.submit("restore", func() error {
		if err := s.apply(v, false); err != nil {
			return err
		}
		return s.startEnvironment(false)
	})
}

func (s *Service) UpdateComponents() (Job, error) {
	return s.submit("environment-update", s.synchronizeEnvironment)
}

func (s *Service) UpdateTools() (Job, error) {
	return s.submit("environment-tools", s.upgradeEnvironmentTools)
}
