package workbench

import (
	"errors"
	"net/http"

	"github.com/sylvinus/agent-vm/internal/browser"
)

func (s *Server) browser(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	if r.Method == "GET" {
		action = "tabs"
	}
	if !browser.Supports(action) {
		http.NotFound(w, r)
		return
	}
	var input browser.Input
	if r.Method != "GET" {
		if !readJSON(w, r, &input) {
			return
		}
	}
	var data []byte
	var err error
	if r.Method == "GET" {
		data, err = s.BrowserTabs(r.Context())
	} else {
		data, err = s.BrowserCommand(r.Context(), action, input)
	}
	if err != nil {
		var upstream *BrowserRequestError
		switch {
		case errors.Is(err, errClosing):
			http.Error(w, err.Error(), 503)
		case errors.Is(err, ErrBrowserUnavailable), errors.Is(err, ErrOperationBusy):
			http.Error(w, err.Error(), 409)
		case errors.As(err, &upstream):
			http.Error(w, err.Error(), 502)
		default:
			requestError(w, err)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *Server) browserEvents(w http.ResponseWriter, r *http.Request) {
	stream, err := s.SubscribeBrowser(r.Context())
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, ErrBrowserUnavailable) {
			status = http.StatusConflict
		} else if errors.Is(err, errClosing) {
			status = http.StatusServiceUnavailable
		}
		http.Error(w, "Browser unavailable", status)
		return
	}
	defer stream.Close()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Connection", "keep-alive")
	flusher := w.(http.Flusher)
	flusher.Flush()
	buffer := make([]byte, 16<<10)
	for {
		n, err := stream.Read(buffer)
		if n > 0 {
			if _, err := w.Write(buffer[:n]); err != nil {
				return
			}
			flusher.Flush()
		}
		if err != nil {
			return
		}
	}
}
