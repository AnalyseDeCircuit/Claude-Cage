package environment

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Use an actual local command to check the CLI boundary: arguments, directory,
// job environment, output and cancellation. No VM or host settings are changed.
func commandRuntime(t *testing.T, failure string) (*Runtime, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	profile := filepath.Join(root, "private")
	if err := os.MkdirAll(filepath.Join(profile, "state"), 0700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "cage-test")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$CLAUDE_CAGE_CONFIG.calls"
printf '%s\n' "$PWD" "$CLAUDE_CAGE_CONFIG" "$CLAUDE_CAGE_JOB_ID"
printf 'command stderr\n' >&2
`
	if failure != "" {
		script += "if [ \"$1\" = " + failure + " ]; then exit 1; fi\n"
	}
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	r := New(profile, filepath.Join(profile, "config.json"), executable)
	if _, err := r.Prepare(""); err != nil {
		t.Fatal(err)
	}
	return r, &bytes.Buffer{}
}

func TestStartCommandSequenceAndFailure(t *testing.T) {
	for _, tc := range []struct {
		name, failure, calls string
		existing, cancel     bool
	}{
		{name: "first-start", calls: "setup\nup\n"},
		{name: "existing-base", existing: true, calls: "up\n"},
		{name: "setup-failure", failure: "setup", calls: "setup\n"},
		{name: "component-failure", failure: "up", calls: "setup\nup\n"},
		{name: "cancel-after-setup", cancel: true, calls: "setup\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, output := commandRuntime(t, tc.failure)
			if tc.existing {
				if err := os.WriteFile(filepath.Join(r.profile, "state", ".agent-vm-base-version"), []byte("existing"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			messages := 0
			err := r.Start(ctx, Operation{JobID: "start-job", Output: output, Progress: func(string) {
				messages++
				if tc.cancel && messages == 2 {
					cancel()
				}
			}})
			if (err != nil) != (tc.failure != "" || tc.cancel) {
				t.Fatalf("unexpected start result: %v", err)
			}
			calls, readErr := os.ReadFile(r.configPath + ".calls")
			if readErr != nil || string(calls) != tc.calls {
				t.Fatalf("commands = %q, want %q: %v", calls, tc.calls, readErr)
			}
			for _, expected := range []string{r.Directory(), r.configPath, "start-job", "command stderr"} {
				if !strings.Contains(output.String(), expected+"\n") {
					t.Fatalf("command output lost %q: %s", expected, output.String())
				}
			}
		})
	}
}

func TestComponentUpdateDoesNotSetUpBaseAndStopRequiresExistingVM(t *testing.T) {
	r, output := commandRuntime(t, "")
	op := Operation{JobID: "update-job", Output: output}
	if err := r.ApplyComponents(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	if err := r.Stop(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(r.configPath + ".calls")
	if err != nil || string(calls) != "up\n" {
		t.Fatalf("component update ran setup or stop ran without a VM: %q, %v", calls, err)
	}
	if err := os.MkdirAll(filepath.Dir(r.instanceFile("lima.yaml")), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.instanceFile("lima.yaml"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.Stop(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	calls, err = os.ReadFile(r.configPath + ".calls")
	if err != nil || string(calls) != "up\nstop\n" {
		t.Fatalf("existing VM was not stopped: %q, %v", calls, err)
	}
}
