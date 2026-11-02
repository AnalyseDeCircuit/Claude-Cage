package projects

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// Mount commands are supplied by the host. Use idle children to exercise the
// manager's connection ownership without requiring a VM or mounting host paths.
type guestStub struct {
	commands int
}

func (*guestStub) Run(context.Context, string) ([]byte, error) { return nil, nil }
func (g *guestStub) Command(ctx context.Context, _ string) *exec.Cmd {
	g.commands++
	return exec.CommandContext(ctx, "cat")
}

func TestShareReuseAndClose(t *testing.T) {
	guest := &guestStub{}
	m := New(t.TempDir(), t.TempDir(), guest)
	t.Cleanup(m.Close)
	a, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{a, b, a} {
		if err := m.EnsureShare(context.Background(), path); err != nil {
			t.Fatal(err)
		}
	}
	if guest.commands != 2 || len(m.shares) != 2 {
		t.Fatal("switching directories replaced an existing connection")
	}
	done := []<-chan struct{}{m.shares[a].done, m.shares[b].done}
	m.Close()
	for _, closed := range done {
		select {
		case <-closed:
		case <-time.After(5 * time.Second):
			t.Fatal("closing shares left a transport child running")
		}
	}
	if err := m.EnsureShare(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if guest.commands != 3 {
		t.Fatal("reopening the environment reused a closed connection")
	}
}

func TestRestoreContinuesAfterMissingDirectoryAndRemembersLegacySelection(t *testing.T) {
	profile, environment := t.TempDir(), t.TempDir()
	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "removed")
	m := New(profile, environment, &guestStub{})
	for _, path := range []string{missing, project} {
		if err := m.Remember(path); err != nil {
			t.Fatal(err)
		}
	}
	// A fresh manager reads the existing profile; current need not be in its grants.
	reloaded := New(profile, environment, &guestStub{})
	t.Cleanup(reloaded.Close)
	current, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reloaded.Restore(context.Background(), current)
	if reloaded.RestoreError() == "" || len(reloaded.shares) != 2 {
		t.Fatal("one unavailable project prevented restoring the other directories")
	}
	reloaded.Close()
	paths, err := reloaded.Paths("")
	if err != nil || !slices.Equal(paths, []string{environment, missing, project, current}) {
		t.Fatalf("restoring or closing lost saved grants: %v, %v", paths, err)
	}
	info, err := os.Stat(filepath.Join(profile, "projects.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("saved project grants are not private")
	}
}
