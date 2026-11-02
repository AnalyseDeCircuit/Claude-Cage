package terminal

import (
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

type Info struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Closed    bool   `json:"closed"`
	Workspace string `json:"workspace"`
}

type Session struct {
	mu        sync.Mutex
	id, title string
	workspace string
	cmd       *exec.Cmd
	file      *os.File
	history   []byte
	closed    bool
	clients   map[chan []byte]bool
	done      chan struct{}
}

func (s *Session) Info() Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Info{ID: s.id, Title: s.title, Closed: s.closed, Workspace: s.workspace}
}

func (s *Session) Write(data []byte) (int, error) { return s.file.Write(data) }

func (s *Session) Resize(cols, rows uint16) error {
	if cols > 1 && cols < 600 && rows > 1 && rows < 400 {
		return pty.Setsize(s.file, &pty.Winsize{Cols: cols, Rows: rows})
	}
	return nil
}

// Subscription captures history and registers live output under the same lock,
// so reconnecting clients cannot miss output between the two. Output chunks are
// read-only. A slow client is disconnected instead of blocking the terminal.
type Subscription struct {
	History []byte
	Output  <-chan []byte
	session *Session
	channel chan []byte
}

func (s *Session) Subscribe() *Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan []byte, 64)
	if s.closed {
		close(ch)
	} else {
		s.clients[ch] = true
	}
	return &Subscription{History: append([]byte(nil), s.history...), Output: ch, session: s, channel: ch}
}

// Close detaches only this client; the process and its output remain available.
func (sub *Subscription) Close() {
	s := sub.session
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients[sub.channel] {
		delete(s.clients, sub.channel)
		close(sub.channel)
	}
}

func (s *Session) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		_ = s.file.Close()
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
	}
}

func (s *Session) read() {
	defer close(s.done)
	b := make([]byte, 16<<10)
	for {
		n, err := s.file.Read(b)
		if n > 0 {
			chunk := append([]byte(nil), b[:n]...)
			s.mu.Lock()
			s.history = append(s.history, chunk...)
			if len(s.history) > 512<<10 {
				s.history = s.history[len(s.history)-(512<<10):]
			}
			for ch := range s.clients {
				select {
				case ch <- chunk:
				default:
					close(ch)
					delete(s.clients, ch)
				}
			}
			s.mu.Unlock()
		}
		if err != nil {
			break
		}
	}
	_ = s.cmd.Wait()
	s.mu.Lock()
	s.closed = true
	for ch := range s.clients {
		close(ch)
		delete(s.clients, ch)
	}
	s.mu.Unlock()
	_ = s.file.Close()
}
