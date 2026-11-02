package workbench

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sylvinus/agent-vm/internal/terminal"
)

func startTestTerminal(t *testing.T, s *Server) *terminal.Session {
	t.Helper()
	session, err := s.terminals.Start(t.TempDir(), exec.Command("cat"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.terminals.Close(session.Info().ID) })
	return session
}

func readTerminalWebSocket(t *testing.T, ws *websocket.Conn, want string) string {
	t.Helper()
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var out strings.Builder
	for !strings.Contains(out.String(), want) {
		kind, data, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("terminal WebSocket ended before %q: %v (%q)", want, err, out.String())
		}
		if kind != websocket.BinaryMessage {
			t.Fatalf("terminal output changed message type: %d", kind)
		}
		out.Write(data)
	}
	return out.String()
}

func TestTerminalWebSocketAuthenticationReplayAndControls(t *testing.T) {
	s := testServer(t)
	session, err := s.terminals.Start(t.TempDir(), exec.Command("sh", "-c", `stty -echo; printf 'initial-output\n'; while IFS= read -r line; do if [ "$line" = size ]; then stty size; else printf 'reply:%s\n' "$line"; fi; done`))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.terminals.CloseAll)
	server := httptest.NewUnstartedServer(s.handler())
	s.origin = "http://" + server.Listener.Addr().String()
	server.Start()
	defer server.Close()
	path := "/api/terminals/" + session.Info().ID
	url := "ws" + strings.TrimPrefix(server.URL, "http") + path + "/stream"
	for _, tc := range []struct {
		origin string
		auth   bool
		status int
	}{{s.origin, false, 401}, {"https://elsewhere.example", true, 403}} {
		headers := http.Header{"Origin": {tc.origin}}
		if tc.auth {
			headers.Set("Cookie", s.cookieName()+"="+s.session)
		}
		ws, response, err := websocket.DefaultDialer.Dial(url, headers)
		if ws != nil {
			_ = ws.Close()
		}
		if response != nil {
			_ = response.Body.Close()
		}
		if err == nil || response == nil || response.StatusCode != tc.status {
			t.Fatalf("untrusted terminal connection accepted: %v, %v", response, err)
		}
	}
	dial := func() *websocket.Conn {
		t.Helper()
		ws, _, err := websocket.DefaultDialer.Dial(url, http.Header{"Origin": {s.origin}, "Cookie": {s.cookieName() + "=" + s.session}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ws.Close() })
		return ws
	}
	first := dial()
	readTerminalWebSocket(t, first, "initial-output")
	if err := first.WriteMessage(websocket.TextMessage, []byte("{")); err != nil {
		t.Fatal(err)
	}
	if err := first.WriteJSON(map[string]any{"type": "resize", "cols": 110, "rows": 35}); err != nil {
		t.Fatal(err)
	}
	if err := first.WriteJSON(map[string]string{"type": "input", "data": "size\n"}); err != nil {
		t.Fatal(err)
	}
	readTerminalWebSocket(t, first, "35 110")
	_ = first.Close()
	second := dial()
	out := readTerminalWebSocket(t, second, "35 110")
	if !strings.Contains(out, "initial-output") || session.Info().Closed {
		t.Fatal("reconnect lost output or restarted the session")
	}
	if err := second.WriteJSON(map[string]string{"type": "input", "data": "after-reconnect\n"}); err != nil {
		t.Fatal(err)
	}
	readTerminalWebSocket(t, second, "reply:after-reconnect")
	s.terminals.CloseAll()
	_ = second.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := second.ReadMessage(); err == nil {
		t.Fatal("stopped terminal left its WebSocket open")
	}
	finished := dial()
	readTerminalWebSocket(t, finished, "reply:after-reconnect")
	_ = finished.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := finished.ReadMessage(); err == nil {
		t.Fatal("finished session stream did not end after replay")
	}

	call := func(method, path string) *http.Response {
		t.Helper()
		r, err := http.NewRequest(method, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Origin", s.origin)
		r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: s.session})
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = response.Body.Close() })
		return response
	}
	response := call("GET", "/api/terminals")
	var list []terminal.Info
	if err := json.NewDecoder(response.Body).Decode(&list); err != nil || response.StatusCode != 200 || len(list) != 1 || list[0] != session.Info() {
		t.Fatalf("terminal summary changed: %+v, %v", list, err)
	}
	for i := 0; i < 2; i++ {
		response = call("DELETE", path)
		var result map[string]bool
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil || response.StatusCode != 200 || !result["ok"] {
			t.Fatalf("terminal close contract changed: %+v, %v", result, err)
		}
	}
	if response = call("GET", path+"/stream"); response.StatusCode != 404 {
		t.Fatalf("removed terminal stream returned %d", response.StatusCode)
	}
}
