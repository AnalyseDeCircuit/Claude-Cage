package workbench

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/jobs"
	"github.com/sylvinus/agent-vm/internal/vmname"
)

type Job = jobs.State

var errClosing = errors.New("后台正在更新，请重新打开 Cage。")

// Caller holds mu. Admission always takes the service lock before the job lock,
// keeping network intent and its pause generation together without moving the
// network state machine into jobs.
func (s *Service) jobQueueLocked() *jobs.Queue {
	if s.jobs == nil {
		s.jobs = jobs.New(&s.op)
	}
	return s.jobs
}

func (s *Service) jobQueue() *jobs.Queue {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jobQueueLocked()
}

func (s *Service) submit(kind string, fn func() error) (Job, error) {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return Job{}, errClosing
	}
	var pauseVersion uint64
	intentFailed := false
	job, err := s.jobQueueLocked().Submit(kind, func() error {
		if kind == "start" || kind == "safety" || kind == "restore" {
			if err := s.saveNetworkIntentLocked(false); err != nil {
				intentFailed = true
				return err
			}
		}
		pauseVersion = s.safety.pauseVersion
		if s.monitorCancel != nil {
			s.monitorCancel()
		}
		return nil
	}, func(*jobs.Execution) error {
		s.mu.Lock()
		s.activePauseVersion = pauseVersion
		s.mu.Unlock()
		defer func() { s.mu.Lock(); s.activePauseVersion = 0; s.mu.Unlock() }()
		return fn()
	})
	s.mu.Unlock()
	if intentFailed {
		_ = s.closeProxyGate("网络恢复意图无法保存，外网保持关闭")
	}
	return job, err
}

func (s *Service) busy() bool { return s.jobQueue().Snapshot().Running }
func (s *Service) setMessage(message string) {
	s.jobQueue().Execution().SetMessage(message)
}
func (s *Service) operationContext() context.Context {
	execution := s.jobQueue().Execution()
	if execution.ID() != "" {
		return execution.Context()
	}
	return s.lifetimeContext()
}

func (s *Service) Stop() (Job, error) {
	_, done, err := s.beginActivity(context.Background())
	if err != nil {
		return Job{}, err
	}
	defer done()
	for _, name := range []string{vmname.Name(s.environmentDir()), "base-building"} {
		if owner := cage.InstanceJobID(filepath.Join(s.profile, "state"), name); owner != "" {
			_ = cage.RequestJobStop(s.profile, owner)
		}
	}
	s.mu.Lock()
	queue := s.jobQueueLocked()
	jobID := queue.Execution().ID()
	if queue.Snapshot().Kind != "stop" {
		queue.Cancel()
	}
	s.mu.Unlock()
	if jobID != "" {
		_ = cage.RequestJobStop(s.profile, jobID)
	}
	_ = s.pauseNetworkNow("正在停止环境")
	return s.submit("stop", s.stopEnvironment)
}
