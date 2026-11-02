package workbench

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestDownloadRetryUsesAuthenticatedFixedGuestAction(t *testing.T) {
	s := testServer(t)
	s.ready = true
	s.credentials.Token = "guest-test-token"
	var called atomic.Int32
	useBrowserService(t, s, func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		if r.URL.Path != "/download-retry" || r.Header.Get("Authorization") != "Bearer guest-test-token" {
			t.Error("retry did not use the authenticated guest action")
		}
		var input struct{ ID string }
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.ID != "download-id" {
			t.Error("retry lost the selected download", err)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	for _, action := range []string{"download-retry", "editor-restart"} {
		if w := request(s, "POST", "/api/browser/"+action, s.origin, `{}`, false); w.Code != 401 {
			t.Fatal("recovery accepted without session", action, w.Code)
		}
		if w := request(s, "POST", "/api/browser/"+action, "https://example.com", `{}`, true); w.Code != 403 {
			t.Fatal("cross-origin recovery accepted", action, w.Code)
		}
		if w := request(s, "POST", "/api/browser/"+action, s.origin, `{"unit":"ssh.service"}`, true); w.Code != 400 {
			t.Fatal("arbitrary service parameters accepted", action, w.Code)
		}
	}
	if called.Load() != 0 {
		t.Fatal("rejected request reached the guest")
	}
	if w := request(s, "POST", "/api/browser/download-retry", s.origin, `{"id":"download-id"}`, true); w.Code != 200 || called.Load() != 1 {
		t.Fatal("authorized retry failed", w.Code, w.Body.String())
	}
}
