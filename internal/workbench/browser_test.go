package workbench

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sylvinus/agent-vm/internal/browser"
)

func useBrowserService(t *testing.T, s *Server, handler http.HandlerFunc) {
	t.Helper()
	guest := httptest.NewServer(handler)
	t.Cleanup(guest.Close)
	s.browserControl = browser.New(guest.URL)
}

func TestBrowserEventsKeepAuthenticationStatusAndStreamingBehavior(t *testing.T) {
	s := testServer(t)
	s.credentials.Token = "guest-token"
	var calls atomic.Int32
	var fail atomic.Bool
	ended := make(chan struct{}, 1)
	payload := ": heartbeat\n\ndata: {\"active\":\"tab-1\",\"tabs\":[],\"downloads\":[],\"future_field\":\"" + strings.Repeat("文", 6000) + "\"}\n\n"
	useBrowserService(t, s, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/events" || r.Header.Get("Authorization") != "Bearer guest-token" || r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" {
			t.Error("event relay changed the guest action or forwarded host credentials")
		}
		if fail.Load() {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(payload))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		ended <- struct{}{}
	})
	if w := request(s, "GET", "/api/browser/events", "", "", false); w.Code != 401 {
		t.Fatal("unauthenticated event subscription accepted", w.Code)
	}
	for _, path := range []string{"/api/browser/events", "/api/browser/tabs"} {
		if w := request(s, "GET", path, "", "", true); w.Code != 409 {
			t.Fatal("unready browser accepted", path, w.Code)
		}
	}
	for _, action := range []string{"audit", "homepage", "unknown"} {
		if w := request(s, "POST", "/api/browser/"+action, s.origin, `{}`, true); w.Code != 404 {
			t.Fatal("private or unknown browser action exposed", action, w.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("rejected request reached the guest")
	}
	s.ready = true
	fail.Store(true)
	if w := request(s, "GET", "/api/browser/events", "", "", true); w.Code != 502 || strings.HasPrefix(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatal("failed guest subscription started a successful event response", w.Code)
	}
	fail.Store(false)
	server := httptest.NewUnstartedServer(s.handler())
	s.origin = "http://" + server.Listener.Addr().String()
	server.Start()
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/browser/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: s.session})
	response, err := server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "text/event-stream" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("event response headers changed", response.StatusCode, response.Header)
	}
	data := make([]byte, len(payload))
	if _, err := io.ReadFull(response.Body, data); err != nil || string(data) != payload {
		t.Fatal("relay changed or buffered SSE data and heartbeats", err)
	}
	_ = response.Body.Close()
	select {
	case <-ended:
	case <-ctx.Done():
		t.Fatal("closing the UI stream did not release the guest subscription")
	}
}
