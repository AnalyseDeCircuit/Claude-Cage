package workbench

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/projects"
	"github.com/sylvinus/agent-vm/internal/vmname"
)

func TestStopClearsConnectionsEvenWithoutVMDefinition(t *testing.T) {
	s, gate := grantedToolUpdateServer(t)
	holdTestJob(t, s, "settings") // An internal restart must not stop the proxy core.
	s.preferences.Workspace = s.environmentDir()
	s.credentials.Token = "guest-test"
	s.guestUpdateRequired, s.guestUpdateError = true, "previous update failed"
	terminal := startTestTerminal(t, s)
	output := terminal.Subscribe()
	defer output.Close()
	if err := s.stopEnvironment(); err != nil {
		t.Fatal(err)
	}
	if !terminal.Info().Closed {
		t.Fatal("missing VM definition skipped terminal cleanup")
	}
	select {
	case _, open := <-output.Output:
		if open {
			t.Fatal("stopped terminal still produced output")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stopped terminal did not finish")
	}
	if _, err := terminal.Write([]byte("after stop")); err == nil {
		t.Fatal("stopped terminal still accepted input")
	}
	if s.ready || s.credentials.Token != "" || s.guestManifest != nil || s.guestUpdateRequired || s.guestUpdateError != "" {
		t.Fatal("stopped environment retained connection or update state")
	}
	if gate.Status().Mode != cage.GateClosed {
		t.Fatal("stop left the network gate open")
	}
}

func TestEnvironmentIdentityDoesNotFollowLastProject(t *testing.T) {
	s := testServer(t)
	s.profile = filepath.Join(t.TempDir(), "private")
	s.preferences.Workspace = t.TempDir()
	previous := s.preferences.Workspace
	if err := s.prepareEnvironment(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(cage.EnvironmentDir(s.profile)) })
	if s.environmentDir() == previous || s.environmentDir() != cage.EnvironmentDir(s.profile) || s.workspace() != previous {
		t.Fatal("environment identity followed the selected project")
	}
	if _, err := projects.ValidateWorkspace(s.environmentDir(), s.profile); err != nil {
		t.Fatal(err)
	}
	var saved Preferences
	b, err := os.ReadFile(filepath.Join(s.profile, "workbench.json"))
	if err != nil || json.Unmarshal(b, &saved) != nil || saved.Workspace != s.workspace() {
		t.Fatal("environment workspace was not persisted")
	}
}

func TestEnvironmentEntryPreservesLiveProject(t *testing.T) {
	s := testServer(t)
	s.preferences.Workspace, _ = filepath.EvalSymlinks(t.TempDir())
	workspace := s.workspace()
	path := filepath.Join(s.profile, "state", "lima", vmname.Name(workspace), "cage-running.json")
	if err := writePrivate(filepath.Join(filepath.Dir(path), "lima.yaml"), map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if err := writePrivate(path, map[string]int{"pid": os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	if err := s.prepareEnvironment(); err != nil {
		t.Fatal(err)
	}
	if s.workspace() != workspace || s.environmentDir() != workspace {
		t.Fatal("live project was replaced")
	}
}

func TestBrowserHomeSavesWithoutRestartAndRoutesNewPages(t *testing.T) {
	s := testServer(t)
	cfg, _ := loadConfig(s.configPath)
	s.ready = true
	s.credentials.Token = "private-guest-token"
	var paths []string
	var pathsMu sync.Mutex
	useBrowserService(t, s, func(w http.ResponseWriter, r *http.Request) {
		pathsMu.Lock()
		paths = append(paths, r.URL.Path)
		pathsMu.Unlock()
		var input struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if input.URL != "https://example.com/start" {
			t.Errorf("incorrect home sent: %q", input.URL)
		}
		_, _ = w.Write([]byte(`{"id":"page"}`))
	})
	p := s.preferences
	p.BrowserHomeURL = "https://example.com/start"
	if err := s.apply(savedProfile{Config: cfg, Preferences: p}, true); err != nil {
		t.Fatal(err)
	}
	if !s.ready {
		t.Fatal("preference change stopped the environment")
	}
	w := request(s, "POST", "/api/browser/new", s.origin, `{}`, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	pathsMu.Lock()
	defer pathsMu.Unlock()
	if strings.Join(paths, ",") != "/homepage,/new" {
		t.Fatalf("unexpected lifecycle or browser requests: %v", paths)
	}
	b, err := os.ReadFile(filepath.Join(s.profile, "workbench.json"))
	var saved Preferences
	if err != nil || json.Unmarshal(b, &saved) != nil || saved.BrowserHomeURL != p.BrowserHomeURL {
		t.Fatal("home was not persisted")
	}
}
