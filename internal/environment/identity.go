package environment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/privatefile"
	"github.com/sylvinus/agent-vm/internal/projects"
	"github.com/sylvinus/agent-vm/internal/vmname"
)

func (r *Runtime) Directory() string { return cage.EnvironmentDir(r.profile) }
func (r *Runtime) instanceFile(name string) string {
	return filepath.Join(r.profile, "state", "lima", vmname.Name(r.Directory()), name)
}

// Prepare retains an existing installation's VM and persists its stable identity.
// The caller saves the selected project separately from this directory.
func (r *Runtime) Prepare(current string) (string, error) {
	path := r.Directory()
	// Promote an existing installation's VM once, preserving its disk and login state.
	if _, err := os.Stat(filepath.Join(r.profile, "environment.json")); os.IsNotExist(err) {
		previous := current
		if previous != "" {
			if _, err := os.Stat(filepath.Join(r.profile, "state", "lima", vmname.Name(previous), "lima.yaml")); err == nil {
				path = previous
			}
		}
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return "", err
	}
	path, err := projects.ValidateWorkspace(path, r.profile)
	if err != nil {
		return "", err
	}
	if err = privatefile.WriteJSON(filepath.Join(r.profile, "environment.json"), map[string]string{"directory": path}); err != nil {
		return "", err
	}
	return path, nil
}

func (r *Runtime) Running() bool {
	profile, workspace := r.profile, r.Directory()
	if workspace == "" {
		return false
	}
	b, e := os.ReadFile(filepath.Join(profile, "state", "lima", vmname.Name(workspace), "cage-running.json"))
	if e != nil {
		return false
	}
	var record struct {
		PID int `json:"pid"`
	}
	if json.Unmarshal(b, &record) != nil || record.PID < 1 {
		return false
	}
	p, e := os.FindProcess(record.PID)
	if e != nil {
		return false
	}
	defer p.Release()
	return p.Signal(syscall.Signal(0)) == nil
}
