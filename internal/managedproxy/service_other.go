//go:build !darwin

package managedproxy

import (
	"context"
	"errors"
	"os"
)

// Preserve the existing unsupported-platform behavior; no fallback process is
// started, and stopping a profile without an installed service remains a no-op.
func Start(context.Context, string, Store) error {
	return errors.New("内置代理当前支持 macOS")
}

func Stop(profile string) error {
	if _, err := os.Stat(servicePath(profile)); os.IsNotExist(err) {
		return nil
	}
	return errors.New("内置代理当前支持 macOS")
}
