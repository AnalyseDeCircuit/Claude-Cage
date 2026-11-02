package workbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sylvinus/agent-vm/internal/browser"
	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/projects"
)

func TestServiceCloseDrainsWorkBeforeReleasingProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile", "config.json")
	service, err := OpenService(path, "http://localhost:17777")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	activity, done, err := service.beginActivity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	finishActivity := sync.OnceFunc(done)
	defer finishActivity()
	started := make(chan struct{})
	_, err = service.submit("test", func() error {
		close(started)
		<-service.operationContext().Done()
		return service.operationContext().Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("job did not start")
	}
	closed := make(chan error, 1)
	go func() { closed <- service.Close() }()
	select {
	case <-activity.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("service did not cancel its activities")
	}
	waitJobFinished(t, service)
	if _, err := service.Start(); !errors.Is(err, errClosing) {
		t.Fatal("closing service accepted work", err)
	}
	select {
	case err := <-closed:
		t.Fatal("service released resources before its activity returned", err)
	case <-time.After(30 * time.Millisecond):
	}
	if other, err := OpenService(path, "http://localhost:17778"); err == nil {
		_ = other.Close()
		t.Fatal("profile lock was released before activity cleanup")
	}
	finishActivity()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("service shutdown did not finish")
	}
	if err := service.Close(); err != nil {
		t.Fatal("repeated close changed its result", err)
	}
	reopened, err := OpenService(path, "http://localhost:17778")
	if err != nil {
		t.Fatal("profile lock was not released", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFailedServiceInitializationReleasesProfile(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile")
	if err := os.MkdirAll(filepath.Join(profile, "workbench.json"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(profile, "config.json")
	if service, err := OpenService(path, "http://localhost:17777"); err == nil {
		_ = service.Close()
		t.Fatal("service accepted an unwritable preference path")
	}
	if err := os.Remove(filepath.Join(profile, "workbench.json")); err != nil {
		t.Fatal(err)
	}
	service, err := OpenService(path, "http://localhost:17777")
	if err != nil {
		t.Fatal("failed initialization retained its profile lock", err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
}

type lifecycleGuest struct{ commands []*exec.Cmd }

func (*lifecycleGuest) Run(context.Context, string) ([]byte, error) { return nil, nil }
func (g *lifecycleGuest) Command(ctx context.Context, _ string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "cat")
	g.commands = append(g.commands, cmd)
	return cmd
}

func TestServiceExitClosesHostResourcesButPreservesVMAndNetworkIntent(t *testing.T) {
	s, gate := interfaceMonitorServer(t)
	s.preferences.Workspace = s.environmentDir()
	if err := s.saveNetworkIntentLocked(false); err != nil {
		t.Fatal(err)
	}
	intentPath := filepath.Join(s.profile, "network-intent.json")
	intent, _ := os.ReadFile(intentPath)
	guest := &lifecycleGuest{}
	s.projects = projects.New(s.profile, s.environmentDir(), guest)
	project, _ := filepath.EvalSymlinks(t.TempDir())
	if err := s.projects.EnsureShare(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.projects.Close)
	cmd := exec.Command("cat")
	if _, err := s.terminals.Start(project, cmd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.terminals.CloseAll)
	holdTestJob(t, s, "test")
	if err := s.Service.Close(); err != nil {
		t.Fatal(err)
	}
	if cmd.ProcessState == nil || len(guest.commands) != 1 || guest.commands[0].ProcessState == nil {
		t.Fatal("shutdown returned before its terminal and share children were reaped")
	}
	if !s.vmRunning() || gate.Status().Mode != cage.GateClosed {
		t.Fatal("controller exit stopped the VM or left its network gate open")
	}
	after, _ := os.ReadFile(intentPath)
	if string(after) != string(intent) || s.networkPaused || s.ready {
		t.Fatal("controller exit changed durable network intent or retained ready state")
	}
}

func TestServiceCloseCancelsAnInFlightMonitor(t *testing.T) {
	service := testService(t)
	service.ready = true
	cfg, _ := loadConfig(service.configPath)
	cfg.ProxyRevision = "unapplied" // No interface recovery participates in this fixture.
	if err := writePrivate(service.configPath, cfg); err != nil {
		t.Fatal(err)
	}
	started, ended := make(chan struct{}), make(chan struct{})
	guest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(ended)
	}))
	defer guest.Close()
	service.browserControl = browser.New(guest.URL)
	// Exercise cancellation during a real health request without competing
	// with the independent network monitor for the operation lock.
	ctx := service.lifetimeContext()
	service.monitors.Add(1)
	go func() { defer service.monitors.Done(); service.monitor(ctx) }()
	defer service.Close()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("monitor did not start its health request")
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ended:
	case <-time.After(3 * time.Second):
		t.Fatal("monitor retained its request after shutdown")
	}
}

func TestServiceExitUnblocksTerminalInput(t *testing.T) {
	service := testService(t)
	session, err := service.terminals.Start(t.TempDir(), exec.Command("sh", "-c", "stty raw -echo; printf ready; exec sleep 30"))
	if err != nil {
		t.Fatal(err)
	}
	defer service.terminals.CloseAll()
	output := session.Subscribe()
	defer output.Close()
	initial := output.History
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for !bytes.Contains(initial, []byte("ready")) {
		select {
		case chunk, open := <-output.Output:
			if !open {
				t.Fatal("terminal exited before entering raw mode")
			}
			initial = append(initial, chunk...)
		case <-deadline.C:
			t.Fatal("terminal did not enter raw mode")
		}
	}
	_, done, err := service.beginActivity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	written := make(chan struct{})
	go func() {
		defer done()
		defer close(written)
		_, _ = session.Write(bytes.Repeat([]byte("x"), 1<<20))
	}()
	select {
	case <-written:
		t.Fatal("fixture did not block terminal input")
	case <-time.After(30 * time.Millisecond):
	}
	closed := make(chan error, 1)
	go func() { closed <- service.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("service waited for terminal input without closing its PTY")
	}
}

func TestInterfaceDisconnectKeepsTerminalButServiceExitClosesStreams(t *testing.T) {
	s := testServer(t)
	s.ready = true
	terminal := startTestTerminal(t, s)
	useBrowserService(t, s, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	server := httptest.NewUnstartedServer(s.handler())
	s.origin = "http://" + server.Listener.Addr().String()
	server.Start()
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/terminals/" + terminal.Info().ID + "/stream"
	dial := func() *websocket.Conn {
		t.Helper()
		ws, _, err := websocket.DefaultDialer.Dial(url, http.Header{"Origin": {s.origin}, "Cookie": {s.cookieName() + "=" + s.session}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ws.Close() })
		return ws
	}
	_ = dial().Close()
	if terminal.Info().Closed || s.lifetimeContext().Err() != nil {
		t.Fatal("interface disconnect ended the terminal or service lifetime")
	}
	ws := dial()
	r, _ := http.NewRequest("GET", server.URL+"/api/browser/events", nil)
	r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: s.session})
	response, err := server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if err := s.Service.Close(); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := ws.ReadMessage(); err == nil {
		t.Fatal("service exit retained a hijacked WebSocket")
	}
	if _, err := response.Body.Read(make([]byte, 1)); err == nil {
		t.Fatal("service exit retained its SSE response")
	}
	if !terminal.Info().Closed {
		t.Fatal("service exit did not finish its terminal")
	}
}

func TestHostedServiceExitsAndReleasesControlRecord(t *testing.T) {
	for _, mode := range []string{"cancel", "handoff"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "profile", "config.json")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			finished := make(chan error, 1)
			go func() { finished <- ServeContext(ctx, path) }()
			var record controlFile
			deadline := time.Now().Add(5 * time.Second)
			for {
				data, err := os.ReadFile(filepath.Join(filepath.Dir(path), "control.json"))
				if err == nil && json.Unmarshal(data, &record) == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("hosted controller did not publish its record")
				}
				time.Sleep(time.Millisecond)
			}
			origin := fmt.Sprintf("http://localhost:%d", record.Port)
			client := &http.Client{Timeout: 3 * time.Second}
			if mode == "handoff" {
				body, _ := json.Marshal(map[string]string{"token": record.Token})
				r, _ := http.NewRequest("POST", origin+"/api/handoff", bytes.NewReader(body))
				r.Header.Set("Origin", origin)
				r.Header.Set("Content-Type", "application/json")
				response, err := client.Do(r)
				if err != nil {
					t.Fatal(err)
				}
				_ = response.Body.Close()
				if response.StatusCode != 200 {
					t.Fatal("idle handoff failed", response.StatusCode)
				}
			} else {
				cancel()
			}
			select {
			case err := <-finished:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("host shutdown deadlocked or retained background work")
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(path), "control.json")); !os.IsNotExist(err) {
				t.Fatal("host shutdown retained a stale control record", err)
			}
			service, err := OpenService(path, origin)
			if err != nil {
				t.Fatal("host shutdown retained its control lock", err)
			}
			if err := service.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
