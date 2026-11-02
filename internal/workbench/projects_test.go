package workbench

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/sylvinus/agent-vm/internal/projects"
	"github.com/sylvinus/agent-vm/internal/vmname"
)

func TestProjectSelectionKeepsOneVMAndExistingSessionsOffline(t *testing.T) {
	s := testServer(t)
	cfg, _ := loadConfig(s.configPath)
	s.preferences.Workspace = s.environmentDir()
	s.ready, s.networkPaused = true, true
	marker := filepath.Join(s.profile, "state", "lima", vmname.Name(s.environmentDir()), "cage-running.json")
	if err := writePrivate(marker, map[string]int{"pid": os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	originalVM := vmname.Name(s.environmentDir())
	terminal := startTestTerminal(t, s)
	a, _ := filepath.EvalSymlinks(t.TempDir())
	b, _ := filepath.EvalSymlinks(t.TempDir())
	useProjectGuest(t, s)
	for _, dir := range []string{a, b, a} {
		next := s.preferences
		next.Workspace = dir
		// The configured proxy is unavailable. Project selection must not probe it.
		if err := s.apply(savedProfile{Config: cfg, Preferences: next}, true); err != nil {
			t.Fatal(err)
		}
		if !s.ready || !s.vmRunning() || !s.networkPaused || vmname.Name(s.environmentDir()) != originalVM {
			t.Fatal("project selection changed environment lifecycle or networking")
		}
		if s.terminals.Get(terminal.Info().ID) != terminal || terminal.Info().Closed {
			t.Fatal("project selection interrupted the old terminal")
		}
	}
	var saved struct {
		Paths []string `json:"paths"`
	}
	data, err := os.ReadFile(filepath.Join(s.profile, "projects.json"))
	if err != nil || json.Unmarshal(data, &saved) != nil || !slices.Equal(saved.Paths, []string{a, b}) {
		t.Fatal("explicitly opened projects were not persisted uniquely")
	}
	next := s.preferences
	next.Workspace = s.profile
	if err := s.apply(savedProfile{Config: cfg, Preferences: next}, true); err == nil {
		t.Fatal("private directory accepted as a project")
	}
	if s.workspace() != a || terminal.Info().Closed || !s.ready {
		t.Fatal("failed project selection changed the active environment")
	}
}

func TestProjectListComesFromSavedDirectories(t *testing.T) {
	s := testServer(t)
	var expected []string
	expected = append(expected, s.environmentDir())
	for i := 0; i < 15; i++ {
		path := t.TempDir()
		if err := s.rememberSharedProject(path); err != nil {
			t.Fatal(err)
		}
		expected = append(expected, path)
	}
	s.preferences.Workspace = expected[2]
	if err := s.rememberSharedProject(expected[2]); err != nil {
		t.Fatal(err)
	}
	// Reload from the same profile, without any browser-local history.
	reloaded := testServer(t)
	reloaded.profile = s.profile
	reloaded.preferences = s.preferences
	w := request(reloaded, "GET", "/api/status", "", "", true)
	var status struct {
		Projects []string `json:"projects"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil || !slices.Equal(status.Projects, expected) {
		t.Fatalf("saved project list was truncated, reordered, or lost: %s", w.Body.String())
	}
}

func TestProjectActionsUseExplicitDirectoryWithoutSwitchingProject(t *testing.T) {
	s := testServer(t)
	a, _ := filepath.EvalSymlinks(t.TempDir())
	b, _ := filepath.EvalSymlinks(t.TempDir())
	s.preferences.Workspace = b
	s.ready = true
	if err := s.rememberSharedProject(a); err != nil {
		t.Fatal(err)
	}
	useProjectGuest(t, s)
	marker := filepath.Join(s.profile, "state", "lima", vmname.Name(s.environmentDir()), "cage-running.json")
	if err := writePrivate(marker, map[string]int{"pid": os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	// The absent SSH config makes this local child exit before connecting.
	body, _ := json.Marshal(map[string]string{"workspace": a})
	w := request(s, "POST", "/api/terminals", s.origin, string(body), true)
	var terminal struct{ ID, Workspace string }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &terminal) != nil || terminal.Workspace != a {
		t.Fatalf("terminal did not use the explicit directory: %s", w.Body.String())
	}
	session := s.terminals.Get(terminal.ID)
	t.Cleanup(func() { s.terminals.Close(terminal.ID) })
	if session == nil || session.Info().Workspace != a {
		t.Fatal("terminal manager received a different directory")
	}
	var editorWorkspace atomic.Value
	useBrowserService(t, s, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Workspace string `json:"workspace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		editorWorkspace.Store(input.Workspace)
		_, _ = w.Write([]byte(`{"id":"editor"}`))
	})
	for _, action := range []string{"editor", "editor-restart"} {
		w = request(s, "POST", "/api/browser/"+action, s.origin, string(body), true)
		if w.Code != 200 || editorWorkspace.Load() != a || s.workspace() != b {
			t.Fatalf("%s changed the selected project or ignored its directory: %s", action, w.Body.String())
		}
	}
	for _, path := range []string{t.TempDir(), s.profile} {
		body, _ = json.Marshal(map[string]string{"workspace": path})
		for _, endpoint := range []string{"/api/terminals", "/api/browser/editor", "/api/browser/editor-restart"} {
			w = request(s, "POST", endpoint, s.origin, string(body), true)
			if w.Code != 400 {
				t.Fatalf("unopened directory accepted by %s: %d", endpoint, w.Code)
			}
		}
	}
	if len(s.terminals.List()) != 1 || s.workspace() != b {
		t.Fatal("rejected requests changed existing sessions")
	}
}

// These integration tests exercise project selection, not the guest mount.
// The manager still owns and closes real transport child processes.
type projectGuestStub struct{}

func (projectGuestStub) Run(context.Context, string) ([]byte, error) { return nil, nil }
func (projectGuestStub) Command(ctx context.Context, _ string) *exec.Cmd {
	return exec.CommandContext(ctx, "cat")
}
func useProjectGuest(t *testing.T, s *Server) {
	t.Helper()
	s.projects = projects.New(s.profile, s.environmentDir(), projectGuestStub{})
	t.Cleanup(s.projects.Close)
}
