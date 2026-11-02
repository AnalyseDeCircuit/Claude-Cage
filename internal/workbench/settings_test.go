package workbench

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sylvinus/agent-vm/internal/settings"
)

func TestSettingsSaveUsesExistingJobQueue(t *testing.T) {
	s := testServer(t)
	cfg, _ := loadConfig(s.configPath)
	input := settings.View(cfg, s.preferences)
	input.Keyboard = "native"
	body, _ := json.Marshal(input)
	s.op.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest("POST", s.origin+"/api/settings", strings.NewReader(string(body))).WithContext(ctx)
	r.Header.Set("Origin", s.origin)
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: s.session})
	w := httptest.NewRecorder()
	s.handler().ServeHTTP(w, r)
	// Closing the submitting request must not cancel an accepted service job.
	cancel()
	if w.Code != 200 {
		s.op.Unlock()
		t.Fatal(w.Code, w.Body.String())
	}
	var queued Job
	if json.Unmarshal(w.Body.Bytes(), &queued) != nil || queued.Kind != "settings" || !queued.Running || queued.ID == "" {
		s.op.Unlock()
		t.Fatal("save no longer returns the existing asynchronous job", w.Body.String())
	}
	if s.preferences.Keyboard != "mac" {
		s.op.Unlock()
		t.Fatal("save bypassed the environment operation lock")
	}
	if w := request(s, "POST", "/api/settings", s.origin, string(body), true); w.Code != 409 {
		s.op.Unlock()
		t.Fatal("another settings save was accepted while the job was queued", w.Code)
	}
	s.op.Unlock()
	deadline := time.Now().Add(3 * time.Second)
	for {
		s.mu.Lock()
		job, preferences := s.jobQueueLocked().Snapshot(), s.preferences
		s.mu.Unlock()
		if !job.Running {
			if job.ID != queued.ID || job.Error != "" || preferences.Keyboard != "native" {
				t.Fatal("queued save failed or published the wrong preferences", job)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("settings job did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	if got := s.settingsStore().ReadPreferences(Preferences{}); got.Keyboard != "native" {
		t.Fatal("successful job did not persist preferences")
	}
}

func TestSettingsStorageFailuresStopBeforeLaterEffects(t *testing.T) {
	for _, failure := range []string{"backup", "preferences"} {
		t.Run(failure, func(t *testing.T) {
			s := testServer(t)
			s.ready = true
			terminal := startTestTerminal(t, s)
			old, _ := loadConfig(s.configPath)
			next := savedProfile{Config: old, Preferences: s.preferences}
			var browserCalls atomic.Int32
			useBrowserService(t, s, func(w http.ResponseWriter, r *http.Request) { browserCalls.Add(1) })
			blocked := "workbench.json"
			if failure == "backup" {
				blocked = "previous-profile.json"
				next.Config.Timezone = "Asia/Tokyo"
			} else {
				next.Preferences.BrowserHomeURL = "https://example.com/"
			}
			if err := os.Mkdir(filepath.Join(s.profile, blocked), 0700); err != nil {
				t.Fatal(err)
			}
			if err := s.apply(next, false); err == nil {
				t.Fatal("storage failure was ignored")
			}
			current, _ := loadConfig(s.configPath)
			if current != old || s.preferences != (Preferences{Keyboard: "mac"}) || !s.ready || terminal.Info().Closed || browserCalls.Load() != 0 {
				t.Fatal("failed settings write changed applied preferences, environment or browser")
			}
		})
	}
}
