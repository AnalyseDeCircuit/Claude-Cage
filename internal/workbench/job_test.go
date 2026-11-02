package workbench

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/environment"
	"github.com/sylvinus/agent-vm/internal/vmname"
)

func waitJobFinished(t *testing.T, s *Service) Job {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		job := s.jobQueue().Snapshot()
		if !job.Running {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatal("job did not finish", job)
		}
		time.Sleep(time.Millisecond)
	}
}

func cancelTestJob(t *testing.T, s *Server) {
	t.Helper()
	s.jobQueue().Cancel()
	waitJobFinished(t, s.Service)
}

// Keep a real job active for tests of busy-state and network rules. No task
// state is injected: cancellation and completion go through the normal queue.
func holdTestJob(t *testing.T, s *Server, kind string) {
	t.Helper()
	cancelTestJob(t, s)
	started := make(chan struct{})
	_, err := s.submit(kind, func() error {
		ctx := s.operationContext()
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancelTestJob(t, s) })
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("test job did not start")
	}
}

func TestStopPreemptsJobWithoutLosingStopResult(t *testing.T) {
	s := testServer(t)
	started, cancelled, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var output environment.Operation
	s.launch(httptest.NewRecorder(), "safety", func() error {
		ctx := s.operationContext()
		output = s.environmentOperation()
		close(started)
		<-ctx.Done()
		close(cancelled)
		s.setMessage("stale result")
		_, _ = output.Output.Write([]byte("stale log"))
		return ctx.Err()
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("job did not start")
	}
	w := httptest.NewRecorder()
	s.launch(w, "stop", func() error { s.setMessage("stopped"); close(finished); return nil })
	if w.Code != 200 {
		t.Fatalf("stop rejected: %d", w.Code)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("old job was not cancelled")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("stop did not run")
	}
	s.op.Lock()
	defer s.op.Unlock()
	// The command writer may outlive the old job's callback. It must stay bound
	// to that job even after the stop has completed and no execution is active.
	output.Progress("late stale progress")
	_, _ = output.Output.Write([]byte("late stale log"))
	job := s.jobQueue().Snapshot()
	if output.JobID == "" || job.Kind != "stop" || job.Running || job.Error != "" || job.Log != "" || job.Message != "已完成" {
		t.Fatalf("stale job overwrote stop: %+v", job)
	}
}

func TestPauseBypassesBusyQueueAndRevokesCheck(t *testing.T) {
	s := testServer(t)
	cfg, err := loadConfig(s.configPath)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(s.profile, "state", "lima", vmname.Name(s.environmentDir()))
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	g, err := cage.NewEgressGate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	stop, err := cage.StartGateControl(s.configPath, dir, g)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if err = g.BeginCheck(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	holdTestJob(t, s, "safety")
	s.checkCancel = cancel
	w := request(s, "POST", "/api/safety/close", s.origin, "{}", true)
	if w.Code != 200 {
		t.Fatalf("pause rejected while checking: %d %s", w.Code, w.Body.String())
	}
	if ctx.Err() == nil || g.Status().Mode != cage.GateClosed || !s.networkPaused {
		t.Fatal("pause did not cancel and close the gate")
	}
}
