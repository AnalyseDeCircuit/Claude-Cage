package terminal_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sylvinus/agent-vm/internal/terminal"
)

func readUntil(t *testing.T, sub *terminal.Subscription, want string) []byte {
	t.Helper()
	out := append([]byte(nil), sub.History...)
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for !bytes.Contains(out, []byte(want)) {
		select {
		case chunk, open := <-sub.Output:
			if !open {
				t.Fatalf("terminal ended before %q: %q", want, out)
			}
			out = append(out, chunk...)
		case <-deadline.C:
			t.Fatalf("terminal did not produce %q: %q", want, out)
		}
	}
	return out
}

func waitForEnd(t *testing.T, sub *terminal.Subscription) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case _, open := <-sub.Output:
			if !open {
				return
			}
		case <-deadline.C:
			t.Fatal("terminal output did not close")
		}
	}
}

func TestSessionReconnectInputAndResize(t *testing.T) {
	var manager terminal.Manager
	t.Cleanup(manager.CloseAll)
	workspace := t.TempDir()
	cmd := exec.Command("sh", "-c", `stty -echo; printf 'ready\n'; while IFS= read -r line; do case "$line" in size) stty size;; quit) exit;; *) printf 'reply:%s\n' "$line";; esac; done`)
	s, err := manager.Start(workspace, cmd)
	if err != nil {
		t.Fatal(err)
	}
	info := s.Info()
	if info.ID == "" || info.Title != "终端 1" || info.Workspace != workspace || info.Closed || !manager.HasActive() {
		t.Fatalf("wrong session metadata: %+v", info)
	}
	first := s.Subscribe()
	readUntil(t, first, "ready")
	first.Close()
	first.Close()
	if _, err := s.Write([]byte("offline\n")); err != nil {
		t.Fatal(err)
	}
	second := s.Subscribe()
	defer second.Close()
	out := readUntil(t, second, "reply:offline")
	if !bytes.Contains(out, []byte("ready")) || s.Info().Closed {
		t.Fatal("disconnect lost history or closed the process")
	}
	// Resize reaches the real PTY; out-of-range sizes leave the last size intact.
	for _, size := range [][2]uint16{{120, 40}, {1, 40}, {600, 40}, {120, 1}, {120, 400}} {
		if err := s.Resize(size[0], size[1]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Write([]byte("size\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, second, "40 120")
	if _, err := s.Write([]byte("quit\n")); err != nil {
		t.Fatal(err)
	}
	waitForEnd(t, second)
	if !s.Info().Closed || manager.HasActive() || !cmd.ProcessState.Success() {
		t.Fatal("natural exit was not observed or reaped")
	}
	last := s.Subscribe()
	defer last.Close()
	if !bytes.Contains(last.History, []byte("reply:offline")) || !bytes.Contains(last.History, []byte("40 120")) {
		t.Fatal("finished session lost replay output")
	}
	waitForEnd(t, last)
}

func TestCloseAllStopsProcessesAndRetainsSessions(t *testing.T) {
	var manager terminal.Manager
	t.Cleanup(manager.CloseAll)
	cmd := exec.Command("sh", "-c", "printf 'retained-output\\n'; exec cat")
	s, err := manager.Start(t.TempDir(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	sub := s.Subscribe()
	defer sub.Close()
	readUntil(t, sub, "retained-output")
	manager.CloseAll()
	manager.CloseAll()
	waitForEnd(t, sub)
	if !s.Info().Closed || manager.HasActive() || len(manager.List()) != 1 || manager.Get(s.Info().ID) != s || cmd.ProcessState == nil {
		t.Fatal("stop did not reap the process and retain its summary")
	}
	if _, err := s.Write([]byte("after close")); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("PTY stayed open: %v", err)
	}
	replay := s.Subscribe()
	defer replay.Close()
	if !bytes.Contains(replay.History, []byte("retained-output")) {
		t.Fatal("stop discarded output history")
	}
}

func TestManagerLimitRemovalAndFailedStart(t *testing.T) {
	var manager terminal.Manager
	t.Cleanup(manager.CloseAll)
	if manager.List() == nil || manager.Get("missing") != nil || manager.HasActive() {
		t.Fatal("empty manager has incorrect state")
	}
	if _, err := manager.Start("", exec.Command(filepath.Join(t.TempDir(), "missing"))); err == nil || len(manager.List()) != 0 {
		t.Fatal("failed start consumed a session")
	}
	for i := 0; i < terminal.MaxSessions; i++ {
		if _, err := manager.Start("workspace", exec.Command("cat")); err != nil {
			t.Fatal(err)
		}
	}
	manager.CloseAll()
	cmd := exec.Command("cat")
	if _, err := manager.Start("workspace", cmd); !errors.Is(err, terminal.ErrLimit) || cmd.Process != nil {
		t.Fatal("retained closed sessions did not enforce the limit before starting a process")
	}
	id := manager.List()[0].ID
	manager.Close(id)
	manager.Close(id)
	if manager.Get(id) != nil || len(manager.List()) != terminal.MaxSessions-1 {
		t.Fatal("explicit close did not remove the session")
	}
	s, err := manager.Start("another workspace", exec.Command("cat"))
	if err != nil || s.Info().Title != "终端 13" || s.Info().ID == id {
		t.Fatalf("removed slot was not reusable: %v", err)
	}
}

func TestHistoryIsBoundedAndSlowSubscriberDoesNotBlockProcess(t *testing.T) {
	var manager terminal.Manager
	t.Cleanup(manager.CloseAll)
	cmd := exec.Command("sh", "-c", `stty -echo; printf 'ready\n'; read line; head -c 2097152 /dev/zero | tr '\000' x; printf 'END'`)
	s, err := manager.Start("workspace", cmd)
	if err != nil {
		t.Fatal(err)
	}
	slow := s.Subscribe()
	defer slow.Close()
	readUntil(t, slow, "ready")
	if _, err := s.Write([]byte("go\n")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for manager.HasActive() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if manager.HasActive() || !cmd.ProcessState.Success() {
		t.Fatal("slow subscriber blocked the terminal reader or process cleanup")
	}
	waitForEnd(t, slow)
	replay := s.Subscribe()
	defer replay.Close()
	if len(replay.History) != 512<<10 || !strings.HasSuffix(string(replay.History), "END") {
		t.Fatalf("wrong retained output: %d bytes", len(replay.History))
	}
}
