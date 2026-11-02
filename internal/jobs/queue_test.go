package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitFinished(t *testing.T, q *Queue) State {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		state := q.Snapshot()
		if !state.Running {
			return state
		}
		if time.Now().After(deadline) {
			t.Fatal("job did not finish", state)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestResultsProgressAndBoundedOutputWithoutHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, message, failure string
		err                    error
	}{
		{"success", "已完成", "", nil},
		{"failure", "操作未完成", "command failed", errors.New("command failed")},
		{"cancelled", "已取消", "", fmt.Errorf("stopped: %w", context.Canceled)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := New(&sync.Mutex{})
			initial, err := q.Submit("test", nil, func(e *Execution) error {
				if e.ID() == "" || e.Context().Err() != nil || q.Execution() != e {
					t.Error("job did not own its execution context")
				}
				e.SetMessage("running command")
				if q.Snapshot().Message != "running command" {
					t.Error("progress was not published")
				}
				_, _ = e.Write([]byte(strings.Repeat("x", 24001)))
				_, _ = e.Write([]byte("end"))
				return tc.err
			})
			if err != nil || len(initial.ID) != 12 || initial.Kind != "test" || !initial.Running || initial.Message != "正在准备…" {
				t.Fatalf("wrong admission state: %+v, %v", initial, err)
			}
			result := waitFinished(t, q)
			if result.ID != initial.ID || result.Message != tc.message || result.Error != tc.failure || result.Log != strings.Repeat("x", 23997)+"end" {
				t.Fatalf("wrong final state: %+v", result)
			}
			if e := q.Execution(); e.ID() != "" || e.Context().Err() != nil {
				t.Fatal("finished execution leaked into background controls")
			}
		})
	}
}

// The real mutex is also held by synchronous service operations. Observing
// attempts and exits lets tests wait for superseded goroutines to leave it.
type observedOperation struct {
	mu              sync.Mutex
	entered, exited chan struct{}
}

func (o *observedOperation) Lock()   { o.entered <- struct{}{}; o.mu.Lock() }
func (o *observedOperation) Unlock() { o.mu.Unlock(); o.exited <- struct{}{} }

func await(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("job operation did not reach the expected point")
	}
}

func TestQueuedStopSupersedesPendingWorkAndIsIdempotent(t *testing.T) {
	op := &observedOperation{entered: make(chan struct{}, 2), exited: make(chan struct{}, 2)}
	op.mu.Lock()
	unlock := sync.OnceFunc(op.mu.Unlock)
	defer unlock()
	q := New(op)
	var ranOld, ranStop atomic.Int32
	prepared := 0
	prepare := func() error { prepared++; return nil }
	old, err := q.Submit("settings", prepare, func(*Execution) error { ranOld.Add(1); return nil })
	if err != nil {
		t.Fatal(err)
	}
	await(t, op.entered)
	if _, err := q.Submit("start", prepare, func(*Execution) error { return nil }); !errors.Is(err, ErrBusy) || prepared != 1 {
		t.Fatal("busy admission ran preparation or accepted another job", err)
	}
	stop, err := q.Submit("stop", prepare, func(*Execution) error { ranStop.Add(1); return nil })
	if err != nil || stop.ID == old.ID {
		t.Fatal("stop did not supersede the queued job", err)
	}
	await(t, op.entered)
	again, err := q.Submit("stop", prepare, func(*Execution) error { ranStop.Add(100); return nil })
	if err != nil || again != stop || prepared != 2 {
		t.Fatal("repeated stop created work or repeated preparation", err)
	}
	unlock()
	await(t, op.exited)
	await(t, op.exited)
	if result := q.Snapshot(); result.ID != stop.ID || result.Running || ranOld.Load() != 0 || ranStop.Load() != 1 {
		t.Fatal("superseded queued work ran or changed the stop result", result)
	}
}

func TestCancellationBeforeExecutionSkipsTheCallback(t *testing.T) {
	op := &sync.Mutex{}
	op.Lock()
	unlock := sync.OnceFunc(op.Unlock)
	defer unlock()
	q := New(op)
	_, err := q.Submit("settings", nil, func(*Execution) error {
		t.Error("cancelled queued job executed")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	q.Cancel()
	unlock()
	if result := waitFinished(t, q); result.Message != "已取消" || result.Error != "" {
		t.Fatal("queued cancellation was not completed", result)
	}
}

func TestRejectedPreparationDoesNotPublishAJob(t *testing.T) {
	q := New(&sync.Mutex{})
	rejected := errors.New("intent could not be saved")
	_, err := q.Submit("start", func() error { return rejected }, func(*Execution) error {
		t.Error("rejected job executed")
		return nil
	})
	if !errors.Is(err, rejected) || q.Snapshot() != (State{}) {
		t.Fatal("preparation failure changed the queue", err, q.Snapshot())
	}
}

func TestPreemptedExecutionCannotWriteIntoLaterJobs(t *testing.T) {
	q := New(&sync.Mutex{})
	oldExecution := make(chan *Execution, 1)
	_, err := q.Submit("settings", nil, func(e *Execution) error {
		oldExecution <- e
		<-e.Context().Done()
		e.SetMessage("stale progress during cancellation")
		_, _ = e.Write([]byte("stale output during cancellation"))
		return e.Context().Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Cancel()
	var old *Execution
	select {
	case old = <-oldExecution:
	case <-time.After(3 * time.Second):
		t.Fatal("old job did not start")
	}
	stop, err := q.Submit("stop", nil, func(e *Execution) error {
		_, _ = e.Write([]byte("stop output"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result := waitFinished(t, q)
	old.SetMessage("stale progress after completion")
	_, _ = old.Write([]byte("stale output after completion"))
	if result.ID != stop.ID || result.Message != "已完成" || result.Error != "" || result.Log != "stop output" || q.Snapshot() != result || old.Context().Err() != context.Canceled {
		t.Fatal("old execution changed a newer job", q.Snapshot())
	}
}

func TestCloseCancelsWorkDrainsWorkersAndRejectsSubmissions(t *testing.T) {
	for _, pending := range []bool{false, true} {
		name := "running"
		if pending {
			name = "pending"
		}
		t.Run(name, func(t *testing.T) {
			op := &sync.Mutex{}
			if pending {
				op.Lock()
			}
			unlock := sync.OnceFunc(func() {
				if pending {
					op.Unlock()
				}
			})
			defer unlock()
			q := New(op)
			started := make(chan struct{})
			_, err := q.Submit("settings", nil, func(e *Execution) error {
				close(started)
				<-e.Context().Done()
				return e.Context().Err()
			})
			if err != nil {
				t.Fatal(err)
			}
			if !pending {
				await(t, started)
			}
			closed := make(chan struct{})
			go func() { q.Close(); close(closed) }()
			deadline := time.Now().Add(3 * time.Second)
			for {
				q.mu.Lock()
				closing := q.closed
				q.mu.Unlock()
				if closing {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("queue did not begin closing")
				}
				time.Sleep(time.Millisecond)
			}
			if _, err := q.Submit("stop", nil, func(*Execution) error { return nil }); !errors.Is(err, ErrClosed) {
				t.Fatal("closed queue accepted new work", err)
			}
			if pending {
				select {
				case <-closed:
					t.Fatal("close returned with a worker still waiting for the operation lock")
				default:
				}
			}
			unlock()
			await(t, closed)
			q.Close()
			if state := q.Snapshot(); state.Running || state.Message != "已取消" {
				t.Fatal("close did not complete the cancelled job", state)
			}
			if pending {
				select {
				case <-started:
					t.Fatal("shutdown ran a previously pending callback")
				default:
				}
			}
		})
	}
}
