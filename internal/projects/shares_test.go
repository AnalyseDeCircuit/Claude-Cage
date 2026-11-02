package projects

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/lima-vm/sshocker/pkg/reversesshfs"
	"github.com/pkg/sftp"
	"github.com/sylvinus/agent-vm/internal/mounts"
)

func TestLiveProjectTransportConfinesReadsAndProtectsMetadata(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "secret")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(parent, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	server, conn := net.Pipe()
	done := make(chan struct{})
	go func() { defer close(done); _ = reversesshfs.ServeConfined(server, root, false, mounts.BaseNames) }()
	client, err := sftp.NewClientPipe(conn, conn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close(); _ = conn.Close(); <-done }()
	file, err := client.Create(filepath.Join(root, "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write([]byte("shared")); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	data, err := os.ReadFile(filepath.Join(root, "note.txt"))
	if err != nil || string(data) != "shared" {
		t.Fatal("project write did not reach the host")
	}
	for _, p := range []string{outside, filepath.Join(root, "escape", "secret")} {
		f, err := client.Open(p)
		if err == nil {
			_ = f.Close()
			t.Fatalf("read escaped selected project: %s", p)
		}
	}
	if f, err := client.Create(filepath.Join(root, ".git", "config")); err == nil {
		_ = f.Close()
		t.Fatal("Git metadata was writable")
	}
	f, err := client.Open(filepath.Join(root, "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil || string(got) != "shared" {
		t.Fatal("shared file read failed")
	}
}
