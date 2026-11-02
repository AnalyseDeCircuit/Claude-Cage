package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sylvinus/agent-vm/internal/terminal"
)

func (s *Server) listTerminals(w http.ResponseWriter, r *http.Request) {
	jsonReply(w, s.Terminals())
}

func (s *Server) newTerminal(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Workspace string `json:"workspace"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	info, err := s.CreateTerminal(r.Context(), input.Workspace)
	if err == nil {
		jsonReply(w, info)
		return
	}
	switch {
	case errors.Is(err, errClosing):
		http.Error(w, err.Error(), 503)
	case errors.Is(err, ErrOperationBusy), errors.Is(err, ErrTerminalUnavailable), errors.Is(err, terminal.ErrLimit):
		http.Error(w, err.Error(), 409)
	case errors.Is(err, ErrTerminalStart):
		http.Error(w, err.Error(), 500)
	default:
		requestError(w, err)
	}
}

func (s *Server) closeTerminal(w http.ResponseWriter, r *http.Request) {
	if err := s.CloseTerminal(r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	jsonReply(w, map[string]bool{"ok": true})
}

func (s *Server) terminalStream(w http.ResponseWriter, r *http.Request) {
	t := s.Terminal(r.PathValue("id"))
	if t == nil {
		http.NotFound(w, r)
		return
	}
	ws, err := s.upgrade(w, r)
	if err != nil {
		return
	}
	defer ws.Close()
	stopClosing := context.AfterFunc(r.Context(), func() { _ = ws.Close() })
	defer stopClosing()
	ws.SetReadLimit(1 << 20)
	sub := t.Subscribe()
	defer sub.Close()
	if len(sub.History) > 0 {
		if err := ws.WriteMessage(websocket.BinaryMessage, sub.History); err != nil {
			return
		}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for chunk := range sub.Output {
			_ = ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if ws.WriteMessage(websocket.BinaryMessage, chunk) != nil {
				break
			}
		}
		_ = ws.Close()
	}()
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			break
		}
		var v struct {
			Type string `json:"type"`
			Data string `json:"data"`
			Cols uint16 `json:"cols"`
			Rows uint16 `json:"rows"`
		}
		if json.Unmarshal(data, &v) != nil {
			continue
		}
		switch v.Type {
		case "input":
			_, _ = t.Write([]byte(v.Data))
		case "resize":
			_ = t.Resize(v.Cols, v.Rows)
		}
	}
	sub.Close()
	<-done
}
