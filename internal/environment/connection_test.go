package environment

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/vmname"
)

func TestTerminalCommandUsesStableVMAndQuotedWorkspace(t *testing.T) {
	r := New(t.TempDir(), "", "")
	cmd := r.TerminalCommand("/tmp/project 'with quotes'; echo wrong")
	want := []string{"-tt", "lima-" + vmname.Name(r.Directory()), "--", `cd '/tmp/project '"'"'with quotes'"'"'; echo wrong' && exec env TERM=xterm-256color COLORTERM=truecolor CAGE_WORKBENCH=1 bash -l`}
	if !slices.Equal(cmd.Args[len(cmd.Args)-len(want):], want) {
		t.Fatalf("terminal changed its destination or shell command: %q", cmd.Args)
	}
	if cmd.Path != "/usr/bin/ssh" || cmd.Env == nil || !slices.Contains(cmd.Args, "PermitLocalCommand=no") || !slices.Contains(cmd.Args, "ForwardAgent=no") || !strings.Contains(strings.Join(cmd.Args, " "), "ssh.config") {
		t.Fatal("terminal bypassed the restricted guest connection")
	}
}

func TestGuestConnectionRequiresSuccessfulMatchingManifest(t *testing.T) {
	required := cage.RequiredGuestManifest()
	for _, name := range []string{"current", "missing", "old-revision", "old-protocol", "broken"} {
		t.Run(name, func(t *testing.T) {
			manifest := required
			if name == "old-revision" {
				manifest.Revision = "previous"
			}
			if name == "old-protocol" {
				manifest.Protocol++
			}
			encoded, _ := json.Marshal(manifest)
			if name == "missing" {
				encoded = []byte("null")
			}
			if name == "broken" {
				encoded = []byte("{")
			}
			connection, err := parseConnection(append([]byte("{\"token\":\"guest-test\"}\npassword\n"), encoded...))
			if name == "current" {
				if err != nil || connection.Manifest == nil || connection.Credentials.Token != "guest-test" || connection.Credentials.VNCPassword != "password" {
					t.Fatal(connection, err)
				}
			} else if !errors.Is(err, ErrUpdateRequired) {
				t.Fatalf("stale guest accepted: %v", err)
			}
		})
	}
}
