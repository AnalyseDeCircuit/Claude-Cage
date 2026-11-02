package workbench

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBrowserTextRequiresSessionAndExplicitTarget(t *testing.T) {
	s := testServer(t)
	s.ready = true
	s.credentials.Token = "guest-test-token"
	var calls atomic.Int32
	useBrowserService(t, s, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/insert-text" || r.Header.Get("Authorization") != "Bearer guest-test-token" {
			t.Error("wrong text destination or authentication")
		}
		var input struct {
			ID   string
			Text string
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.ID != "tab-1" || input.Text != "是，大奥🙂" {
			t.Error("text or target changed", err)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	path := "/api/browser/insert-text"
	valid := `{"id":"tab-1","text":"是，大奥🙂"}`
	if w := request(s, "POST", path, s.origin, valid, false); w.Code != 401 {
		t.Fatal("unauthenticated input accepted", w.Code)
	}
	if w := request(s, "POST", path, "https://example.com", valid, true); w.Code != 403 {
		t.Fatal("cross-origin input accepted", w.Code)
	}
	for _, body := range []string{`{"text":"是"}`, `{"id":"tab-1"}`, `{"id":"tab-1","text":""}`, `{"id":"tab-1","text":"` + strings.Repeat("中", 3000) + `"}`} {
		if w := request(s, "POST", path, s.origin, body, true); w.Code != 400 {
			t.Fatal("invalid input accepted", w.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("rejected input reached guest")
	}
	if w := request(s, "POST", path, s.origin, valid, true); w.Code != 200 || calls.Load() != 1 {
		t.Fatal("valid input rejected", w.Code, w.Body.String())
	}
}
