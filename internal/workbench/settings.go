// Package workbench hosts Cage's control service and authenticated local APIs.
// Guest web content runs in the VM, never in the host interface.
package workbench

import (
	"net/http"

	"github.com/sylvinus/agent-vm/internal/privatefile"
	"github.com/sylvinus/agent-vm/internal/settings"
)

// Keep the existing workbench types available to its callers.
type Preferences = settings.Preferences
type Settings = settings.Values
type savedProfile = settings.Profile

func (s *Service) settingsStore() *settings.Store { return settings.New(s.profile, s.configPath) }
func (s *Service) workspace() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.preferences.Workspace
}
func writePrivate(path string, v any) error { return privatefile.WriteJSON(path, v) }

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	value, err := s.ReadSettings()
	if err != nil {
		requestError(w, err)
		return
	}
	jsonReply(w, value)
}
func (s *Server) probe(w http.ResponseWriter, r *http.Request) {
	var input Settings
	if !readJSON(w, r, &input) {
		return
	}
	loc, err := s.ProbeSettings(r.Context(), input)
	if err != nil {
		requestError(w, err)
		return
	}
	jsonReply(w, loc)
}
func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	var input Settings
	if !readJSON(w, r, &input) {
		return
	}
	job, err := s.SaveSettings(input)
	jobReply(w, job, err)
}
func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	job, err := s.RestoreSettings()
	jobReply(w, job, err)
}
