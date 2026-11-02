package workbench

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

func (s *Server) vnc(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	ready := s.ready
	s.mu.Unlock()
	if !ready {
		http.Error(w, "环境尚未就绪", 409)
		return
	}
	dialer := net.Dialer{Timeout: 3 * time.Second}
	remote, e := dialer.DialContext(r.Context(), "tcp", "127.0.0.1:15900")
	if e != nil {
		http.Error(w, "画面连接暂时不可用", 502)
		return
	}
	defer remote.Close()
	ws, e := s.upgrade(w, r)
	if e != nil {
		return
	}
	// Closing both ends also interrupts a blocked write to the guest.
	stopClosing := context.AfterFunc(r.Context(), func() { _ = ws.Close(); _ = remote.Close() })
	ws.SetReadLimit(4 << 20)
	done := make(chan struct{})
	defer func() {
		stopClosing()
		_ = ws.Close()
		_ = remote.Close()
		<-done
	}()
	go func() {
		defer close(done)
		defer ws.Close()
		b := make([]byte, 64<<10)
		for {
			n, e := remote.Read(b)
			if n > 0 {
				_ = ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if ws.WriteMessage(websocket.BinaryMessage, b[:n]) != nil {
					return
				}
			}
			if e != nil {
				return
			}
		}
	}()
	for {
		kind, b, e := ws.ReadMessage()
		if e != nil {
			return
		}
		if kind == websocket.BinaryMessage {
			if _, e = remote.Write(b); e != nil {
				return
			}
		}
	}
}
