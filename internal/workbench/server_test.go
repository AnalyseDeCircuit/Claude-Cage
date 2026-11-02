package workbench

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/managedproxy"
	"github.com/sylvinus/agent-vm/internal/settings"
	"github.com/sylvinus/agent-vm/internal/vmname"
)

func testService(t *testing.T) *Service {
	t.Helper()
	profile := t.TempDir()
	path := filepath.Join(profile, "config.json")
	c := cage.DefaultConfig()
	p, _ := managedproxy.Load(profile)
	p.Auth = strings.Repeat("secret", 8)
	if err := p.Import("test", "", []byte(`proxies: [{name: qa, type: socks5, server: 127.0.0.1, port: 1}]`), "source", 0, ""); err != nil {
		t.Fatal(err)
	}
	p.Selected = p.Sources[0].Nodes[0].ID
	p.Revision = "test-revision"
	if err := p.Save(profile); err != nil {
		t.Fatal(err)
	}
	c.ProxyURL = p.Endpoint()
	c.ProxyRevision = p.Revision
	if e := writePrivate(path, c); e != nil {
		t.Fatal(e)
	}
	return &Service{profile: profile, configPath: path, preferences: Preferences{Keyboard: "mac"}}
}

func testServer(t *testing.T) *Server {
	t.Helper()
	service := testService(t)
	service.origin = "http://localhost:17777"
	return &Server{Service: service, token: strings.Repeat("a", 64), session: strings.Repeat("b", 64)}
}
func request(s *Server, method, path, origin, body string, auth bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, s.origin+path, strings.NewReader(body))
	r.Host = "localhost:17777"
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	r.Header.Set("Content-Type", "application/json")
	if auth {
		r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: s.session})
	}
	w := httptest.NewRecorder()
	s.handler().ServeHTTP(w, r)
	return w
}
func TestHostManagementBoundary(t *testing.T) {
	s := testServer(t)
	if w := request(s, "GET", "/api/settings", "", "", false); w.Code != 401 {
		t.Fatalf("unauthenticated settings: %d", w.Code)
	}
	for _, origin := range []string{"", "http://127.0.0.1:18080", "https://attacker.example"} {
		if w := request(s, "POST", "/api/start", origin, "{}", true); w.Code != 403 {
			t.Fatalf("cross-origin %q: %d", origin, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "http://rebind.example:17777/api/status", nil)
	r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: s.session})
	w := httptest.NewRecorder()
	s.handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("DNS rebinding Host accepted: %d", w.Code)
	}
	w = request(s, "GET", "/api/settings", "", "", true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), strings.Repeat("secret", 8)) || strings.Contains(w.Body.String(), "proxy_url") {
		t.Fatal("password returned to UI")
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS unexpectedly enabled")
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-src 'none'") {
		t.Fatal("guest frames allowed")
	}
}
func TestBootstrapRequiresSecretAndIssuesHostOnlyCookie(t *testing.T) {
	s := testServer(t)
	if w := request(s, "POST", "/api/session", s.origin, `{"token":"wrong"}`, false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request(s, "POST", "/api/session", s.origin, `{"token":"`+s.token+`"}{}`, false); w.Code != 400 {
		t.Fatal("extra JSON accepted")
	}
	ticket := s.issueTicket()
	w := request(s, "POST", "/api/session", s.origin, `{"token":"`+ticket+`"}`, false)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if replay := request(s, "POST", "/api/session", s.origin, `{"token":"`+ticket+`"}`, false); replay.Code != 401 {
		t.Fatal("bootstrap ticket may be replayed")
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing cookie")
	}
	c := cookies[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Domain != "" {
		t.Fatalf("unsafe cookie attributes: %+v", c)
	}
}
func TestInvalidSettingsDoNotReplaceCurrentProfile(t *testing.T) {
	s := testServer(t)
	before, _ := os.ReadFile(s.configPath)
	old, _ := cage.Load(s.configPath)
	input := settings.View(old, s.preferences)
	input.Timezone = "Invented/Invalid"
	body, _ := json.Marshal(input)
	w := request(s, "POST", "/api/settings", s.origin, string(body), true)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	after, _ := os.ReadFile(s.configPath)
	if string(before) != string(after) {
		t.Fatal("invalid save changed profile")
	}
}
func TestKeyboardPreferenceDoesNotRestartOrChangeNetwork(t *testing.T) {
	s := testServer(t)
	old, _ := cage.Load(s.configPath)
	s.ready = true
	p := s.preferences
	p.Keyboard = "native"
	if e := s.apply(savedProfile{Config: old, Preferences: p}, true); e != nil {
		t.Fatal(e)
	}
	after, _ := cage.Load(s.configPath)
	if after != old || !s.ready {
		t.Fatal("keyboard-only save changed runtime")
	}
	b, _ := os.ReadFile(filepath.Join(s.profile, "workbench.json"))
	var saved Preferences
	_ = json.Unmarshal(b, &saved)
	if saved.Keyboard != "native" {
		t.Fatal("preference not persisted")
	}
	st, _ := os.Stat(s.configPath)
	if st.Mode().Perm() != 0600 {
		t.Fatal("profile permissions")
	}
}

func TestLocationProbeHasNoDirectFallback(t *testing.T) {
	c := cage.DefaultConfig()
	c.ProxyURL = "socks5://127.0.0.1:1"
	if _, e := probeLocation(context.Background(), c); e == nil {
		t.Fatal("location probe bypassed unavailable proxy")
	}
}
func TestLocationLiveIntegration(t *testing.T) {
	path := os.Getenv("CAGE_TEST_CONFIG")
	if path == "" {
		t.Skip("explicit proxy profile required")
	}
	c, e := cage.Load(path)
	if e != nil {
		t.Fatal(e)
	}
	loc, e := probeLocation(context.Background(), c)
	if e != nil {
		t.Fatal(e)
	}
	if loc.IP == "" || loc.Timezone == "" {
		t.Fatal("missing location")
	}
	t.Log("Proxy geolocation returned a valid IP and IANA timezone; sensitive values omitted")
}

func TestRestoreCannotReplaceManagedEndpoint(t *testing.T) {
	s := testServer(t)
	s.ready = true
	old, _ := cage.Load(s.configPath)
	bad := old
	bad.ProxyURL = "socks5://127.0.0.1:1"
	if e := s.apply(savedProfile{Config: bad, Preferences: s.preferences}, true); e != nil {
		t.Fatal(e)
	}
	after, _ := loadConfig(s.configPath)
	if after != old || !s.ready {
		t.Fatal("restore changed the managed endpoint or interrupted the environment")
	}
}

func TestSSHIsolationOverridesLocalClientSettings(t *testing.T) {
	if _, err := os.Stat("/usr/bin/ssh"); err != nil {
		t.Skip("OpenSSH is required")
	}
	s := testServer(t)
	s.preferences.Workspace = t.TempDir()
	dir := filepath.Join(s.profile, "state", "lima", vmname.Name(s.environmentDir()))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	config := "Host *\n  BatchMode no\n  ForwardAgent yes\n  ForwardX11 yes\n  PermitLocalCommand yes\n  EscapeChar ~\n  PasswordAuthentication yes\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh.config"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CAGE_TEST_HOST_SECRET", "must-not-cross-the-boundary")
	cmd := s.sshCommand(context.Background(), "-G", "lima-test")
	if cmd.Env == nil {
		t.Fatal("SSH inherits the host environment")
	}
	for _, v := range cmd.Env {
		if strings.HasPrefix(v, "CAGE_TEST_HOST_SECRET=") {
			t.Fatal("host secret forwarded")
		}
	}
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	effective := strings.ToLower(string(out))
	for _, expected := range []string{"batchmode yes", "forwardagent no", "forwardx11 no", "permitlocalcommand no", "escapechar none", "passwordauthentication no"} {
		if !strings.Contains(effective, expected) {
			t.Errorf("SSH effective configuration lacks %q", expected)
		}
	}
}
func TestImplicitHostTimezonesAreRejected(t *testing.T) {
	for _, zone := range []string{"", "Local"} {
		c := cage.DefaultConfig()
		c.Timezone = zone
		if c.Validate() == nil {
			t.Errorf("implicit timezone %q accepted", zone)
		}
	}
}
