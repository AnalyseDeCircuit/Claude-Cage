package workbench

import (
	"context"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
)

var loadConfig = cage.Load
var defaultConfig = cage.DefaultConfig

// Interface recovery uses the ordinary cancellable job queue. Location changes
// remain governed by the saved exit policy and never replace its baseline.
func (s *Service) monitorNetwork(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if s.op.TryLock() {
			s.monitorProxyInterface()
			s.checkProxyHealth()
			s.monitorSafety()
			s.refreshProxySubscriptions()
			s.op.Unlock()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
