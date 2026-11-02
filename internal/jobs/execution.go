package jobs

import "context"

// Execution binds context, progress and output to the job that created it.
// Retaining an old writer cannot change a newer job's message or log.
type Execution struct {
	queue *Queue
	id    string
	ctx   context.Context
}

func (e *Execution) ID() string               { return e.id }
func (e *Execution) Context() context.Context { return e.ctx }

func (e *Execution) SetMessage(message string) {
	q := e.queue
	q.mu.Lock()
	defer q.mu.Unlock()
	if e.id != "" && e.id != q.state.ID {
		return
	}
	q.state.Message = message
}

func (e *Execution) Write(b []byte) (int, error) {
	q := e.queue
	q.mu.Lock()
	defer q.mu.Unlock()
	if e.id != "" && e.id != q.state.ID {
		return len(b), nil
	}
	q.state.Log += string(b)
	if len(q.state.Log) > 24000 {
		q.state.Log = q.state.Log[len(q.state.Log)-24000:]
	}
	return len(b), nil
}
