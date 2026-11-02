package projects

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/lima-vm/sshocker/pkg/reversesshfs"
	"github.com/sylvinus/agent-vm/internal/mounts"
)

func newChallenge() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

type shareTransport struct {
	io.ReadCloser
	io.WriteCloser
}

func (p shareTransport) Close() error { _ = p.ReadCloser.Close(); return p.WriteCloser.Close() }

type share struct {
	cmd       *exec.Cmd
	transport shareTransport
	done      chan struct{}
	once      sync.Once
}

func (p *share) close() {
	p.once.Do(func() {
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		if p.transport.ReadCloser != nil {
			_ = p.transport.Close()
		}
	})
}

// Called under the manager lock. Each SFTP server is rooted in exactly the
// directory the user opened; switching projects keeps existing mounts alive.
func (m *Manager) ensureShare(ctx context.Context, path string) error {
	path, err := ValidateWorkspace(path, m.profile)
	if err != nil {
		return err
	}
	if path == m.environment {
		return nil
	}
	current := m.shares[path]
	if current != nil {
		select {
		case <-current.done:
			current.close()
		default:
			return nil
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	quoted := shellescape.Quote(path)
	// Stale FUSE mounts can remain after a control-service restart. Only reconnect
	// this explicitly authorized path, without changing other projects' mounts.
	if _, err = m.guest.Run(ctx, "if mountpoint -q "+quoted+"; then fusermount3 -uz "+quoted+"; fi; sudo mkdir -p "+quoted+" && sudo chown \"$(id -u):$(id -g)\" "+quoted); err != nil {
		return errors.New("无法在 Cage 中准备项目目录")
	}
	probe, err := os.CreateTemp(path, ".cage-mount-probe-*")
	if err != nil {
		return fmt.Errorf("无法验证项目目录的读写权限：%w", err)
	}
	defer os.Remove(probe.Name())
	challenge := newChallenge()
	if _, err = probe.WriteString(challenge); err != nil {
		_ = probe.Close()
		return err
	}
	if err = probe.Close(); err != nil {
		return err
	}
	script := "exec sshfs " + shellescape.Quote(":"+path) + " " + quoted + " -o slave,allow_other,cache=no,nodev,nosuid"
	cmd := m.guest.Command(context.Background(), script)
	output, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	connection := &share{cmd: cmd, transport: shareTransport{output, input}, done: make(chan struct{})}
	if err = cmd.Start(); err != nil {
		_ = connection.transport.Close()
		return err
	}
	m.transports.Add(1)
	go func() {
		defer m.transports.Done()
		_ = reversesshfs.ServeConfined(connection.transport, path, false, mounts.BaseNames)
		_ = connection.transport.Close()
		_ = cmd.Wait()
		close(connection.done)
	}()
	for {
		if _, err = m.guest.Run(ctx, "mountpoint -q "+quoted+" && test \"$(cat "+shellescape.Quote(probe.Name())+")\" = "+shellescape.Quote(challenge)); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			connection.close()
			return errors.New("项目文件夹连接超时，原有环境与会话保持运行")
		case <-connection.done:
			connection.close()
			return errors.New("项目文件夹未能连接，原有环境与会话保持运行")
		case <-time.After(100 * time.Millisecond):
		}
	}
	if m.shares == nil {
		m.shares = map[string]*share{}
	}
	m.shares[path] = connection
	return nil
}
