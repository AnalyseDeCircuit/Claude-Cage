package browser

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSubscriptionsPreserveSSEAndCloseIndependently(t *testing.T) {
	ended := make(chan string, 2)
	const payload = ": heartbeat\n\ndata: {\"active\":\"tab-1\",\"tabs\":[{\"title\":\"是，大奥🙂\"}],\"downloads\":[],\"focus\":true}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"ready":true}`))
			return
		}
		if r.URL.Path != "/events" || r.Method != "GET" || r.Header.Get("Origin") != "" {
			t.Error("subscription changed its guest request")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(payload))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		ended <- r.Header.Get("Authorization")
	}))
	defer server.Close()
	client := New(server.URL)
	defer client.commands.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	first, err := client.Subscribe(ctx, "first-token")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	secondCtx, cancelSecond := context.WithCancel(ctx)
	defer cancelSecond()
	second, err := client.Subscribe(secondCtx, "second-token")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	read := func(stream io.Reader) {
		t.Helper()
		data := make([]byte, len(payload))
		if _, err := io.ReadFull(stream, data); err != nil || string(data) != payload {
			t.Fatalf("SSE payload or heartbeat changed: %q, %v", data, err)
		}
	}
	read(first)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case token := <-ended:
		if token != "Bearer first-token" {
			t.Fatal("closing one subscription closed another or reused its token")
		}
	case <-ctx.Done():
		t.Fatal("closing subscription did not release its guest connection")
	}
	read(second)
	if err := client.Health(ctx, "second-token"); err != nil {
		t.Fatal("closing subscription disrupted browser commands", err)
	}
	cancelSecond()
	if _, err := second.Read(make([]byte, 1)); err == nil {
		t.Fatal("cancelled subscription remained readable")
	}
	select {
	case token := <-ended:
		if token != "Bearer second-token" {
			t.Fatal("subscription did not use the current token")
		}
	case <-ctx.Done():
		t.Fatal("cancellation did not release guest connection")
	}
}
