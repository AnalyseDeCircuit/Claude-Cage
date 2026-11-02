package environment

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/vmname"
)

// Credentials are private host-to-guest controls, never part of public status.
type Credentials struct {
	Token       string `json:"token"`
	VNCPassword string `json:"-"`
}

type Connection struct {
	Credentials Credentials
	Manifest    *cage.GuestManifest
}

func (r *Runtime) SSHCommand(ctx context.Context, args ...string) *exec.Cmd {
	name := vmname.Name(r.Directory())
	cfg := filepath.Join(r.profile, "state", "lima", name, "ssh.config")
	base := []string{"-q", "-e", "none", "-F", cfg, "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes", "-o", "PermitLocalCommand=no", "-o", "PasswordAuthentication=no", "-o", "KbdInteractiveAuthentication=no", "-o", "ProxyCommand=none", "-o", "ProxyJump=none", "-o", "ConnectTimeout=3", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=2", "-o", "ForwardAgent=no", "-o", "IdentityAgent=none", "-o", "ForwardX11=no", "-o", "ClearAllForwardings=yes", "-o", "ControlPath=none"}
	base = append(base, args...)
	cmd := exec.CommandContext(ctx, "/usr/bin/ssh", base...)
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LANG=C", "LC_ALL=C", "TERM=xterm-256color"}
	return cmd
}

// TerminalCommand opens an interactive shell in an already authorized workspace.
// It uses the same restricted SSH configuration as other guest commands.
func (r *Runtime) TerminalCommand(workspace string) *exec.Cmd {
	script := "cd " + shellescape.Quote(workspace) + " && exec env TERM=xterm-256color COLORTERM=truecolor CAGE_WORKBENCH=1 bash -l"
	return r.SSHCommand(context.Background(), "-tt", "lima-"+vmname.Name(r.Directory()), "--", script)
}

func (r *Runtime) Guest(ctx context.Context, script string) ([]byte, error) {
	name := vmname.Name(r.Directory())
	cmd := r.SSHCommand(ctx, "-T", "lima-"+name, "--", script)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(pipe, (64<<10)+1))
	if readErr != nil || len(data) > 64<<10 {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, errors.New("guest control response exceeded its limit")
	}
	if err = cmd.Wait(); err != nil {
		return nil, err
	}
	return data, nil
}

// Connection reads credentials and checks the installed component manifest.
// A non-nil result means the snapshot was read, even if its contents are stale.
// The caller checks browser health before publishing the environment as ready.
func (r *Runtime) Connection(ctx context.Context) (*Connection, error) {
	if !r.Running() {
		return nil, errors.New("环境尚未启动")
	}
	data, err := r.Guest(ctx, guestConnectionSnapshot)
	if err != nil {
		return nil, errors.New("环境尚未准备好")
	}
	connection, err := parseConnection(data)
	return &connection, err
}

var ErrUpdateRequired = errors.New("环境组件需要更新，请先保存浏览器和编辑器中的修改。工作台终端与磁盘保留，编辑器内终端可能结束。")

const guestConnectionSnapshot = `sudo cat /etc/claude-cage/browser-control.json; printf '\n'; cat "$HOME/.config/claude-cage/browser-password"; printf '\n'; if sudo test -f /etc/claude-cage/components.json; then sudo cat /etc/claude-cage/components.json; else printf 'null'; fi`

func parseConnection(data []byte) (Connection, error) {
	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 3)
	var credentials Credentials
	if len(lines) != 3 || json.Unmarshal([]byte(lines[0]), &credentials) != nil || credentials.Token == "" || strings.TrimSpace(lines[1]) == "" {
		return Connection{Credentials: credentials}, errors.New("浏览器控制服务尚未安装")
	}
	credentials.VNCPassword = strings.TrimSpace(lines[1])
	var manifest *cage.GuestManifest
	if json.Unmarshal([]byte(lines[2]), &manifest) != nil || manifest == nil || !manifest.Current() {
		return Connection{credentials, manifest}, ErrUpdateRequired
	}
	return Connection{credentials, manifest}, nil
}
