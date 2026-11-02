package browser

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestControllerCommandsKeepMethodsCredentialsAndPayloads(t *testing.T) {
	type received struct {
		method, path, token, contentType, body string
	}
	requests := make(chan received, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- received{r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), string(body)}
		if r.Header.Get("Origin") != "" || r.Header.Get("Cookie") != "" {
			t.Error("host session headers crossed the guest boundary")
		}
		_, _ = w.Write([]byte(`{"ok":true,"guest_field":"preserved"}`))
	}))
	defer server.Close()
	client := New(server.URL)
	t.Cleanup(client.commands.CloseIdleConnections)
	ctx := context.Background()
	check := func(method, path, body string, result []byte, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if result != nil && string(result) != `{"ok":true,"guest_field":"preserved"}` {
			t.Fatalf("guest response was changed: %s", result)
		}
		got := <-requests
		if got.method != method || got.path != path || got.token != "Bearer guest-token" || got.body != body {
			t.Fatalf("wrong controller request: %+v", got)
		}
		if body != "" && got.contentType != "application/json" {
			t.Fatal("command lost its JSON content type")
		}
	}
	data, err := client.Tabs(ctx, "guest-token")
	check("GET", "/tabs", "", data, err)
	check("GET", "/health", "", nil, client.Health(ctx, "guest-token"))
	check("POST", "/homepage", `{"url":"https://example.com/"}`, nil, client.SetHome(ctx, "guest-token", "https://example.com/"))
	data, err = client.Audit(ctx, "guest-token", false, "Etc/UTC")
	check("POST", "/audit", `{"network":false,"timezone":"Etc/UTC"}`, data, err)
	for _, action := range []string{"tabs", "new", "activate", "close", "navigate", "back", "forward", "reload", "editor", "editor-restart", "download-retry", "resize", "insert-text"} {
		input := Input{ID: "tab-1", URL: "https://example.com/", Width: 1200, Height: 800, Workspace: "/tmp/project", Text: "是，大奥🙂"}
		data, err = client.Command(ctx, "guest-token", action, input)
		check("POST", "/"+action, `{"id":"tab-1","url":"https://example.com/","width":1200,"height":800,"workspace":"/tmp/project","text":"是，大奥🙂"}`, data, err)
	}
	if _, err := client.Command(ctx, "guest-token", "../audit", Input{}); !errors.Is(err, ErrUnknownAction) {
		t.Fatal("arbitrary controller path accepted")
	}
	if _, err := client.Command(ctx, "guest-token", "insert-text", Input{Text: "missing target"}); !errors.Is(err, ErrInvalidText) {
		t.Fatal("direct command bypassed text target validation")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCommandTimeoutsStaySpecificToEachOperation(t *testing.T) {
	client := New("http://127.0.0.1:18500")
	if client.commands.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("guest control uses an environment proxy")
	}
	client.commands.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		want := 5 * time.Second
		switch r.URL.Path {
		case "/audit":
			want = 20 * time.Second
		case "/editor-restart":
			want = 30 * time.Second
		}
		deadline, ok := r.Context().Deadline()
		remaining := time.Until(deadline)
		if !ok || remaining > want || remaining < want-time.Second {
			t.Errorf("%s deadline = %s, want %s", r.URL.Path, remaining, want)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{}}, nil
	})
	ctx := context.Background()
	_, _ = client.Audit(ctx, "token", true, "Etc/UTC")
	_, _ = client.Command(ctx, "token", "editor-restart", Input{Workspace: "/tmp/project"})
	_, _ = client.Tabs(ctx, "token") // A long operation must not change later timeouts.
}

func TestControllerFailuresDoNotExposeGuestErrorsOrFollowRedirects(t *testing.T) {
	for _, mode := range []string{"status", "truncated", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			var redirected atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/elsewhere" {
					redirected.Add(1)
					return
				}
				switch mode {
				case "status":
					w.WriteHeader(400)
					_, _ = w.Write([]byte(`{"error":"private guest diagnostic"}`))
				case "truncated":
					w.Header().Set("Content-Length", "100")
					_, _ = w.Write([]byte("{"))
				case "redirect":
					http.Redirect(w, r, "/elsewhere", http.StatusFound)
				}
			}))
			defer server.Close()
			client := New(server.URL)
			defer client.commands.CloseIdleConnections()
			_, err := client.Tabs(context.Background(), "token")
			want := map[string]string{"status": "浏览器操作失败，请刷新标签后重试", "truncated": "浏览器响应中断", "redirect": "浏览器服务暂时无法连接"}[mode]
			if err == nil || err.Error() != want || redirected.Load() != 0 {
				t.Fatalf("wrong failure behavior: %v, redirects: %d", err, redirected.Load())
			}
			if mode != "truncated" {
				stream, err := client.Subscribe(context.Background(), "token")
				if stream != nil {
					_ = stream.Close()
				}
				if err == nil || err.Error() != "Browser unavailable" || redirected.Load() != 0 {
					t.Fatalf("event stream accepted failure or redirect: %v", err)
				}
			}
		})
	}
}
