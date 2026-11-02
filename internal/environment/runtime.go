// Package environment manages the persistent VM and its installed components.
// Network authorization and workbench sessions belong to their existing owners.
package environment

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
)

// Runtime addresses one private profile. It has no UI or network-policy state.
type Runtime struct {
	profile, configPath, executable string
}

func New(profile, configPath, executable string) *Runtime {
	return &Runtime{profile: profile, configPath: configPath, executable: executable}
}

// Operation carries the existing job identity, output and progress reporting.
type Operation struct {
	JobID    string
	Output   io.Writer
	Progress func(string)
}

func (op Operation) message(text string) {
	if op.Progress != nil {
		op.Progress(text)
	}
}

func (r *Runtime) Run(ctx context.Context, op Operation, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.executable, args...)
	cmd.Dir = r.Directory()
	if cmd.Dir == "" {
		cmd.Dir = r.profile
	}
	cmd.Env = append(os.Environ(), cage.ConfigEnv+"="+r.configPath)
	cmd.Env = append(cmd.Env, cage.JobEnv+"="+op.JobID)
	cmd.Stdout = op.Output
	cmd.Stderr = op.Output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s 未完成，请查看操作详情并重试", args[0])
	}
	return nil
}

// Start prepares the base image if needed, then applies and starts guest services.
// The caller completes host/network checks before entering this operation.
func (r *Runtime) Start(ctx context.Context, op Operation) error {
	if _, err := os.Stat(filepath.Join(r.profile, "state", ".agent-vm-base-version")); os.IsNotExist(err) {
		op.message("首次准备 Linux 环境与开发工具，可能需要数分钟…")
		if err := r.Run(ctx, op, "setup"); err != nil {
			return err
		}
	}
	op.message("正在启动浏览器、编辑器与共享文件…")
	return r.ApplyComponents(ctx, op)
}

// ApplyComponents shares the same idempotent installer with ordinary startup.
// It does not close terminals, rebuild the disk or authorize network access.
func (r *Runtime) ApplyComponents(ctx context.Context, op Operation) error {
	return r.Run(ctx, op, "up")
}

func (r *Runtime) Stop(ctx context.Context, op Operation) error {
	if _, err := os.Stat(r.instanceFile("lima.yaml")); os.IsNotExist(err) {
		return nil
	}
	return r.Run(ctx, op, "stop")
}
