// Package jobs serializes control operations independently of HTTP and UI code.
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
)

var ErrBusy = errors.New("另一个操作仍在进行，请稍候")
var ErrClosed = errors.New("作业调度器已关闭")

type State struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Running bool   `json:"running"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
	Log     string `json:"log,omitempty"`
}

// Queue accepts one pending or running job. A stop supersedes other jobs;
// repeated stops return the same job. operation is the service's existing lock,
// also used by synchronous operations and monitoring.
type Queue struct {
	mu        sync.Mutex
	operation sync.Locker
	state     State
	cancel    context.CancelFunc
	active    *Execution
	closed    bool
	workers   sync.WaitGroup
}

func New(operation sync.Locker) *Queue { return &Queue{operation: operation} }

// Submit admits a job before waiting asynchronously for the operation lock.
// prepare runs only for an accepted job, under the admission lock. It must not
// call Queue methods; the service uses it to record its own operation intent.
// Job lifetime is independent of the HTTP request that submitted it.
func (q *Queue) Submit(kind string, prepare func() error, run func(*Execution) error) (State, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return State{}, ErrClosed
	}
	if q.state.Running {
		if kind == "stop" && q.state.Kind == "stop" {
			return q.state, nil
		}
		if kind != "stop" {
			return State{}, ErrBusy
		}
		q.cancel()
	}
	if prepare != nil {
		if err := prepare(); err != nil {
			return State{}, err
		}
	}
	var id [6]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	q.cancel = cancel
	q.state = State{ID: hex.EncodeToString(id[:]), Kind: kind, Running: true, Message: "正在准备…"}
	execution := &Execution{queue: q, id: q.state.ID, ctx: ctx}
	q.workers.Add(1)
	go func() {
		defer q.workers.Done()
		q.run(execution, cancel, run)
	}()
	return q.state, nil
}

func (q *Queue) run(execution *Execution, cancel context.CancelFunc, run func(*Execution) error) {
	defer cancel()
	q.operation.Lock()
	defer q.operation.Unlock()
	q.mu.Lock()
	if q.state.ID != execution.id {
		q.mu.Unlock()
		return
	}
	if execution.ctx.Err() != nil {
		q.finishLocked(execution, execution.ctx.Err())
		q.mu.Unlock()
		return
	}
	q.active = execution
	q.mu.Unlock()
	err := run(execution)
	q.mu.Lock()
	defer q.mu.Unlock()
	q.finishLocked(execution, err)
}

func (q *Queue) finishLocked(execution *Execution, err error) {
	if q.active == execution {
		q.active = nil
	}
	if q.state.ID != execution.id {
		return
	}
	q.cancel = nil
	q.state.Running = false
	switch {
	case errors.Is(err, context.Canceled):
		q.state.Message = "已取消"
	case err != nil:
		q.state.Error = err.Error()
		q.state.Message = "操作未完成"
	default:
		q.state.Message = "已完成"
	}
}

func (q *Queue) Snapshot() State {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.state
}

func (q *Queue) Cancel() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.cancel != nil {
		q.cancel()
	}
}

// Close rejects further submissions, cancels accepted work and waits for every
// worker, including superseded jobs still waiting for the operation lock.
func (q *Queue) Close() {
	q.mu.Lock()
	q.closed = true
	if q.cancel != nil {
		q.cancel()
	}
	q.mu.Unlock()
	q.workers.Wait()
}

// Execution returns the active job, even if a stop has already superseded its
// published state. Outside a job, controls retain their background context.
func (q *Queue) Execution() *Execution {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.active != nil {
		return q.active
	}
	return &Execution{queue: q, ctx: context.Background()}
}
