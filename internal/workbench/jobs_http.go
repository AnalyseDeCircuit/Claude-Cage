package workbench

import (
	"errors"
	"net/http"

	"github.com/sylvinus/agent-vm/internal/jobs"
)

func (s *Server) launch(w http.ResponseWriter, kind string, fn func() error) {
	job, err := s.submit(kind, fn)
	jobReply(w, job, err)
}

func jobReply(w http.ResponseWriter, job Job, err error) {
	switch {
	case err == nil:
		jsonReply(w, job)
	case errors.Is(err, errClosing):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	case errors.Is(err, jobs.ErrBusy):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		requestError(w, err)
	}
}
