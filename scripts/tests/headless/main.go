// Headless acceptance against a real Cage executable and a disposable VM.
// No host browser, React page, Swift app, fake guest or alternative service is used.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/gorilla/websocket"
	"github.com/sylvinus/agent-vm/internal/workbench"
)

type check struct {
	Name     string        `json:"name"`
	Passed   bool          `json:"passed"`
	Duration time.Duration `json:"duration_ns"`
	Error    string        `json:"error,omitempty"`
}

type report struct {
	Started    time.Time `json:"started_at"`
	Finished   time.Time `json:"finished_at"`
	Passed     bool      `json:"passed"`
	Root       string    `json:"test_root"`
	Executable string    `json:"executable"`
	Checks     []check   `json:"checks"`
	Requests   []string  `json:"requested_paths"`
	ImageCache string    `json:"image_cache,omitempty"`
	Cleanup    string    `json:"cleanup"`
}

type controller struct {
	cmd     *exec.Cmd
	done    chan struct{}
	exitErr error
	log     *os.File
	origin  string
	token   string
	client  *http.Client
}

type acceptance struct {
	ctx                            context.Context
	binary, root, profile, project string
	env                            []string
	imageCache                     string
	service                        *controller
	report                         report
	requests                       map[string]bool
}

func main() {
	binary := flag.String("cage", "_output/bin/cage", "Cage executable built from the candidate source")
	upstream := flag.String("upstream", "", "authorized local SOCKS5 upstream, e.g. socks5://127.0.0.1:7897")
	output := flag.String("report", "", "report directory; defaults to docs/test-reports/headless-<time>")
	imageCache := flag.String("image-cache", "", "optional Lima image cache to copy into the isolated macOS HOME")
	timeout := flag.Duration("timeout", 40*time.Minute, "overall acceptance timeout including first VM setup")
	flag.Parse()
	if err := run(*binary, *upstream, *output, *imageCache, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(binary, upstream, output, imageCache string, timeout time.Duration) (returned error) {
	u, err := url.Parse(upstream)
	if err != nil || u.Scheme != "socks5" || u.User != nil || !net.ParseIP(u.Hostname()).IsLoopback() || u.Port() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("pass an explicitly authorized local SOCKS5 endpoint with no credentials, using -upstream")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return errors.New("invalid upstream port")
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return err
	}
	if st, err := os.Stat(binary); err != nil || !st.Mode().IsRegular() || st.Mode()&0111 == 0 {
		return errors.New("build the candidate Cage executable first")
	}
	if output == "" {
		output = filepath.Join("docs", "test-reports", "headless-"+time.Now().Format("20060102-150405"))
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(output, 0700); err != nil {
		return err
	}
	// /tmp is deliberately short: Lima's Unix sockets have a path length limit.
	root, err := os.MkdirTemp("/tmp", "cage-hl-")
	if err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	lifetime, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(lifetime, timeout)
	defer cancel()
	a := &acceptance{ctx: ctx, binary: binary, root: root, profile: filepath.Join(root, "p"), project: filepath.Join(root, "project"), requests: map[string]bool{}}
	a.report = report{Started: time.Now(), Root: root, Executable: binary}
	if imageCache != "" {
		a.imageCache, err = filepath.Abs(imageCache)
		if err != nil {
			return err
		}
		a.report.ImageCache = a.imageCache
	}
	defer func() {
		cleanupErr := a.cleanup()
		returned = errors.Join(returned, cleanupErr)
		a.report.Finished = time.Now()
		a.report.Passed = returned == nil
		if cleanupErr != nil {
			a.report.Cleanup = cleanupErr.Error()
		} else {
			a.report.Cleanup = "test VM and controller stopped; private profile and disk retained for inspection"
		}
		for path := range a.requests {
			a.report.Requests = append(a.report.Requests, path)
		}
		sort.Strings(a.report.Requests)
		data, err := json.MarshalIndent(a.report, "", "  ")
		if err == nil {
			err = os.WriteFile(filepath.Join(output, "result.json"), append(data, '\n'), 0600)
		}
		returned = errors.Join(returned, err)
		fmt.Println("Acceptance report:", filepath.Join(output, "result.json"))
		fmt.Println("Private test environment:", root)
	}()
	if err := a.step("isolated configuration, HOME and project", a.prepare); err != nil {
		return err
	}
	if err := a.step("control service starts without a window and authenticates", a.startController); err != nil {
		return err
	}
	if err := a.step("unauthenticated service requests are refused", func() error {
		r, _ := http.NewRequestWithContext(ctx, "GET", a.service.origin+"/api/status", nil)
		client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}}
		defer client.CloseIdleConnections()
		response, err := client.Do(r)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != 401 {
			return fmt.Errorf("expected 401, got %d", response.StatusCode)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := a.step("isolated managed proxy uses the authorized upstream", func() error {
		content := fmt.Sprintf("proxies:\n  - name: headless-local-upstream\n    type: socks5\n    server: %s\n    port: %d\n", u.Hostname(), port)
		if err := a.job("/api/proxy/import", map[string]any{"name": "headless acceptance", "content": content}); err != nil {
			return err
		}
		var proxy struct {
			Sources []struct {
				Nodes []struct {
					ID string `json:"id"`
				} `json:"nodes"`
			} `json:"sources"`
		}
		if err := a.call("GET", "/api/proxy", nil, &proxy); err != nil {
			return err
		}
		if len(proxy.Sources) != 1 || len(proxy.Sources[0].Nodes) != 1 {
			return errors.New("import did not retain the single test node")
		}
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return err
		}
		proxyPort := listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()
		if err := a.job("/api/proxy/select", map[string]any{"node_id": proxy.Sources[0].Nodes[0].ID, "port": proxyPort}); err != nil {
			return err
		}
		_, err = a.command(ctx, "check")
		return err
	}); err != nil {
		return err
	}
	if err := a.step("real VM starts and passes the existing network checks", func() error {
		if err := a.job("/api/start", map[string]any{}); err != nil {
			return err
		}
		status, err := a.status()
		if err != nil {
			return err
		}
		if !status.Ready || !status.VMRunning || status.Safety.Gate == nil || status.Safety.Gate.Mode != "open" || !status.Safety.Gate.Until.After(time.Now()) {
			return errors.New("VM, browser or network authorization is not ready")
		}
		return nil
	}); err != nil {
		return err
	}
	if err := a.step("project sharing is configured through the service", func() error {
		var settings map[string]any
		if err := a.call("GET", "/api/settings", nil, &settings); err != nil {
			return err
		}
		settings["workspace"] = a.project
		if err := a.job("/api/settings", settings); err != nil {
			return err
		}
		status, err := a.status()
		if err != nil {
			return err
		}
		if status.Workspace != a.project || status.ProjectError != "" {
			return errors.New("project selection or share restoration failed")
		}
		return nil
	}); err != nil {
		return err
	}
	var terminalID string
	if err := a.step("real terminal writes shared files and survives client disconnect", func() error {
		var terminal struct{ ID, Workspace string }
		if err := a.call("POST", "/api/terminals", map[string]string{"workspace": a.project}, &terminal); err != nil {
			return err
		}
		if terminal.ID == "" || terminal.Workspace != a.project {
			return errors.New("terminal did not use the authorized project")
		}
		terminalID = terminal.ID
		ws, err := a.terminal(terminalID)
		if err != nil {
			return err
		}
		defer ws.Close()
		input := "CAGE_ACCEPTANCE_SESSION=retained; printf 'from-terminal\\n' > headless-proof.txt; printf '\\nCAGE_HEADLESS_%s\\n' READY\n"
		if err := ws.WriteJSON(map[string]string{"type": "input", "data": input}); err != nil {
			return err
		}
		if err := terminalOutput(ws, "CAGE_HEADLESS_READY"); err != nil {
			return err
		}
		if data, err := os.ReadFile(filepath.Join(a.project, "headless-proof.txt")); err != nil || string(data) != "from-terminal\n" {
			return errors.New("terminal output was not written through the real project share")
		}
		input = "(sleep 2; printf 'after-disconnect\\n' >> headless-proof.txt) & printf '\\nDETACH_%s\\n' READY\n"
		if err := ws.WriteJSON(map[string]string{"type": "input", "data": input}); err != nil {
			return err
		}
		if err := terminalOutput(ws, "DETACH_READY"); err != nil {
			return err
		}
		_ = ws.Close()
		if err := until(ctx, 10*time.Second, func() (bool, error) {
			data, err := os.ReadFile(filepath.Join(a.project, "headless-proof.txt"))
			return strings.Contains(string(data), "after-disconnect\n"), err
		}); err != nil {
			return err
		}
		ws, err = a.terminal(terminalID)
		if err != nil {
			return err
		}
		defer ws.Close()
		if err := terminalOutput(ws, "CAGE_HEADLESS_READY"); err != nil {
			return err
		}
		if err := ws.WriteJSON(map[string]string{"type": "input", "data": "printf '\\nRECONNECTED_%s\\n' \"$CAGE_ACCEPTANCE_SESSION\"\n"}); err != nil {
			return err
		}
		return terminalOutput(ws, "RECONNECTED_retained")
	}); err != nil {
		return err
	}
	var editorID string
	if err := a.step("editor opens the project and browser events reconnect without a UI", func() error {
		var editor struct {
			ID string `json:"id"`
		}
		if err := a.call("POST", "/api/browser/editor", map[string]string{"workspace": a.project}, &editor); err != nil {
			return err
		}
		if editor.ID == "" {
			return errors.New("editor command returned no tab")
		}
		editorID = editor.ID
		for i := 0; i < 2; i++ {
			if err := a.browserEvent(editorID); err != nil {
				return err
			}
		}
		out, err := a.command(ctx, "run", "bash", "-c", "test \"$(cat "+shellescape.Quote(filepath.Join(a.project, "headless-proof.txt"))+")\" = \"$(printf 'from-terminal\\nafter-disconnect')\" && curl --noproxy '*' -fsS http://127.0.0.1:18080/healthz >/dev/null && printf 'EDITOR_HEALTH_OK'")
		if err != nil {
			return err
		}
		if !bytes.Contains(out, []byte("EDITOR_HEALTH_OK")) {
			return errors.New("editor health or shared content check failed inside the VM")
		}
		return a.authenticate()
	}); err != nil {
		return err
	}
	if err := a.step("controller exit retains the VM and releases its lock and host sessions", func() error {
		if err := a.signalController(); err != nil {
			return err
		}
		out, err := a.command(ctx, "list")
		if err != nil {
			return err
		}
		if !bytes.Contains(out, []byte("Running")) {
			return errors.New("controller exit unexpectedly stopped the VM")
		}
		if err := a.startController(); err != nil {
			return err
		}
		var terminals []map[string]any
		if err := a.call("GET", "/api/terminals", nil, &terminals); err != nil {
			return err
		}
		if len(terminals) != 0 {
			return errors.New("new controller retained old host terminal sessions")
		}
		if err := a.job("/api/start", map[string]any{}); err != nil {
			return err
		}
		return a.browserEvent(editorID)
	}); err != nil {
		return err
	}
	if err := a.step("stopping the environment closes real terminal, share and VM resources", func() error {
		var terminal struct {
			ID string `json:"id"`
		}
		if err := a.call("POST", "/api/terminals", map[string]string{"workspace": a.project}, &terminal); err != nil {
			return err
		}
		ws, err := a.terminal(terminal.ID)
		if err != nil {
			return err
		}
		defer ws.Close()
		if err := ws.WriteJSON(map[string]string{"type": "input", "data": "cat headless-proof.txt; printf '\\nBEFORE_STOP_%s\\n' READY\n"}); err != nil {
			return err
		}
		if err := terminalOutput(ws, "BEFORE_STOP_READY"); err != nil {
			return err
		}
		if err := a.job("/api/stop", map[string]any{}); err != nil {
			return err
		}
		status, err := a.status()
		if err != nil {
			return err
		}
		if status.Ready || status.VMRunning {
			return errors.New("stop completed with a running environment")
		}
		_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				var timeout net.Error
				if errors.As(err, &timeout) && timeout.Timeout() {
					return errors.New("stopped terminal retained its WebSocket")
				}
				break
			}
		}
		var terminals []struct {
			Closed bool `json:"closed"`
		}
		if err := a.call("GET", "/api/terminals", nil, &terminals); err != nil {
			return err
		}
		for _, session := range terminals {
			if !session.Closed {
				return errors.New("stop retained an active terminal")
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return a.step("idle handoff, service restart and persisted state work without React or Swift", func() error {
		if err := a.handoff(); err != nil {
			return err
		}
		if err := a.startController(); err != nil {
			return err
		}
		status, err := a.status()
		if err != nil {
			return err
		}
		if status.VMRunning || status.Workspace != a.project {
			return errors.New("restart lost persisted project or restarted a stopped VM")
		}
		for path := range a.requests {
			if !strings.HasPrefix(path, "/api/") {
				return errors.New("acceptance loaded an interface resource")
			}
		}
		return a.handoff()
	})
}

func (a *acceptance) prepare() error {
	for _, port := range []int{15900, 16080, 18080, 18500} {
		listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return fmt.Errorf("guest control port %d is occupied; existing services were not changed", port)
		}
		_ = listener.Close()
	}
	for _, dir := range []string{a.profile, a.project, filepath.Join(a.root, "home")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	home := filepath.Join(a.root, "home")
	if a.imageCache != "" {
		// Reuse only this explicit download-cache input. Lima still validates
		// its template's image digest; VM, keys, profile and project are fresh.
		cache := filepath.Join(home, "Library", "Caches", "lima")
		if err := os.CopyFS(cache, os.DirFS(a.imageCache)); err != nil {
			return fmt.Errorf("copying the explicit image cache: %w", err)
		}
	}
	gitConfig := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gitConfig, []byte("[safe]\n\tbareRepository = explicit\n"), 0600); err != nil {
		return err
	}
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		upper := strings.ToUpper(key)
		if key == "HOME" || key == "TMPDIR" || strings.HasPrefix(key, "AGENT_VM_") || strings.HasPrefix(key, "LIMA_") || strings.HasPrefix(key, "GIT_") || key == "CLAUDE_CAGE_CONFIG" || strings.HasSuffix(upper, "_PROXY") || upper == "ANTHROPIC_API_KEY" || upper == "CLAUDE_CODE_OAUTH_TOKEN" {
			continue
		}
		a.env = append(a.env, value)
	}
	a.env = append(a.env, "HOME="+home, "TMPDIR="+a.root, "GIT_CONFIG_GLOBAL="+gitConfig, "GIT_CONFIG_NOSYSTEM=1", "CLAUDE_CAGE_CONFIG="+filepath.Join(a.profile, "config.json"), "AGENT_VM_STATE_DIR="+filepath.Join(a.profile, "state"), "AGENT_VM_LIMA_HOME="+filepath.Join(a.profile, "state", "lima"))
	return nil
}

func (a *acceptance) step(name string, run func() error) error {
	fmt.Println("CHECK:", name)
	started := time.Now()
	err := run()
	entry := check{Name: name, Passed: err == nil, Duration: time.Since(started)}
	if err != nil {
		entry.Error = err.Error()
	}
	a.report.Checks = append(a.report.Checks, entry)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	fmt.Println("PASS:", name)
	return nil
}

func (a *acceptance) startController() error {
	log, err := os.OpenFile(filepath.Join(a.root, "control.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	cmd := exec.Command(a.binary, "serve")
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = a.root, a.env, log, log
	if err := cmd.Start(); err != nil {
		_ = log.Close()
		return err
	}
	c := &controller{cmd: cmd, done: make(chan struct{}), log: log}
	a.service = c
	go func() { c.exitErr = cmd.Wait(); _ = log.Close(); close(c.done) }()
	err = until(a.ctx, 20*time.Second, func() (bool, error) {
		select {
		case <-c.done:
			return false, errors.New("controller exited; inspect the private control.log")
		default:
		}
		data, err := os.ReadFile(filepath.Join(a.profile, "control.json"))
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		var record struct {
			Port  int    `json:"port"`
			Token string `json:"token"`
			PID   int    `json:"pid"`
		}
		if err = json.Unmarshal(data, &record); err != nil {
			return false, err
		}
		if record.PID != cmd.Process.Pid {
			return false, errors.New("control record does not belong to the test child")
		}
		c.origin, c.token = fmt.Sprintf("http://localhost:%d", record.Port), record.Token
		return true, nil
	})
	if err != nil {
		return err
	}
	return a.authenticate()
}

func (a *acceptance) authenticate() error {
	c := a.service
	if c.client != nil {
		c.client.CloseIdleConnections()
	}
	jar, _ := cookiejar.New(nil)
	c.client = &http.Client{Jar: jar, Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	var launch struct {
		URL string `json:"url"`
	}
	if err := a.call("POST", "/api/launch", map[string]string{"token": c.token}, &launch); err != nil {
		return err
	}
	entry, err := url.Parse(launch.URL)
	if err != nil || entry.Scheme+"://"+entry.Host != c.origin || entry.Fragment == "" {
		return errors.New("invalid authenticated service entry")
	}
	return a.call("POST", "/api/session", map[string]string{"token": entry.Fragment}, nil)
}

func (a *acceptance) call(method, path string, input, output any) error {
	a.requests[path] = true
	var body []byte
	if input != nil {
		var err error
		body, err = json.Marshal(input)
		if err != nil {
			return err
		}
	}
	r, err := http.NewRequestWithContext(a.ctx, method, a.service.origin+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	r.Header.Set("Origin", a.service.origin)
	r.Header.Set("Content-Type", "application/json")
	response, err := a.service.client.Do(r)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("%s returned HTTP %d", path, response.StatusCode)
	}
	if output != nil {
		return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output)
	}
	_, err = io.Copy(io.Discard, response.Body)
	return err
}

func (a *acceptance) status() (workbench.Snapshot, error) {
	var status workbench.Snapshot
	err := a.call("GET", "/api/status", nil, &status)
	return status, err
}

func (a *acceptance) job(path string, input any) error {
	var job workbench.Job
	if err := a.call("POST", path, input, &job); err != nil {
		return err
	}
	if job.ID == "" {
		return errors.New("service accepted no job")
	}
	last := ""
	return until(a.ctx, 35*time.Minute, func() (bool, error) {
		status, err := a.status()
		if err != nil {
			return false, err
		}
		if status.Job.ID != job.ID {
			return false, errors.New("accepted job was replaced before completion")
		}
		if status.Job.Message != last {
			last = status.Job.Message
			fmt.Println("  " + last)
		}
		if status.Job.Running {
			return false, nil
		}
		data, _ := json.MarshalIndent(status, "", "  ")
		if err := os.WriteFile(filepath.Join(a.root, "job-"+status.Job.Kind+"-"+job.ID+".json"), data, 0600); err != nil {
			return false, err
		}
		if status.Job.Error != "" {
			return false, errors.New(status.Job.Error)
		}
		if status.Job.Message == "已取消" {
			return false, errors.New("job was cancelled")
		}
		return true, nil
	})
}

func (a *acceptance) terminal(id string) (*websocket.Conn, error) {
	path := "/api/terminals/" + url.PathEscape(id) + "/stream"
	a.requests[path] = true
	entry, _ := url.Parse(a.service.origin)
	headers := http.Header{"Origin": {a.service.origin}}
	var cookies []string
	for _, cookie := range a.service.client.Jar.Cookies(entry) {
		cookies = append(cookies, cookie.String())
	}
	headers.Set("Cookie", strings.Join(cookies, "; "))
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	ws, response, err := dialer.DialContext(a.ctx, "ws"+strings.TrimPrefix(a.service.origin, "http")+path, headers)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	return ws, err
}

func terminalOutput(ws *websocket.Conn, want string) error {
	_ = ws.SetReadDeadline(time.Now().Add(20 * time.Second))
	var output []byte
	for !bytes.Contains(output, []byte(want)) {
		kind, data, err := ws.ReadMessage()
		if err != nil {
			return fmt.Errorf("terminal did not produce %s: %w", want, err)
		}
		if kind != websocket.BinaryMessage {
			return errors.New("terminal changed its output protocol")
		}
		output = append(output, data...)
		if len(output) > 1<<20 {
			return errors.New("terminal acceptance output exceeded its limit")
		}
	}
	return nil
}

func (a *acceptance) browserEvent(editorID string) error {
	a.requests["/api/browser/events"] = true
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", a.service.origin+"/api/browser/events", nil)
	response, err := a.service.client.Do(r)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "text/event-stream" {
		return errors.New("browser event subscription failed")
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var snapshot struct {
			Tabs []struct{ ID, Workspace, Kind string } `json:"tabs"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &snapshot); err != nil {
			return err
		}
		for _, tab := range snapshot.Tabs {
			if tab.ID == editorID && tab.Kind == "editor" && tab.Workspace == a.project {
				return nil
			}
		}
	}
	return errors.New("browser stream did not contain the authorized editor tab")
}

func (a *acceptance) command(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, a.binary, args...)
	cmd.Env, cmd.Dir = a.env, a.project
	output, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.WriteFile(filepath.Join(a.root, "command-"+args[0]+".log"), output, 0600)
		return nil, fmt.Errorf("cage %s failed; inspect private command log", args[0])
	}
	return output, nil
}

func (a *acceptance) signalController() error {
	c := a.service
	if err := c.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	return a.waitController(c)
}

func (a *acceptance) handoff() error {
	c := a.service
	if err := a.call("POST", "/api/handoff", map[string]string{"token": c.token}, nil); err != nil {
		return err
	}
	return a.waitController(c)
}

func (a *acceptance) waitController(c *controller) error {
	select {
	case <-c.done:
		c.client.CloseIdleConnections()
		if c.exitErr != nil {
			return errors.New("controller cleanup returned an error; inspect private control.log")
		}
		if _, err := os.Stat(filepath.Join(a.profile, "control.json")); !os.IsNotExist(err) {
			return errors.New("exited controller retained its control record")
		}
		a.service = nil
		return nil
	case <-time.After(20 * time.Second):
		return errors.New("controller did not finish graceful shutdown")
	case <-a.ctx.Done():
		return a.ctx.Err()
	}
}

func (a *acceptance) cleanup() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// Only the freshly created profile is ever addressed, including on failure.
	var cleanupErr error
	if a.service != nil {
		select {
		case <-a.service.done:
			cleanupErr = a.service.exitErr
		default:
			// Cancel active setup/jobs before asking the CLI to take VM locks.
			if err := a.service.cmd.Process.Signal(syscall.SIGTERM); err != nil {
				cleanupErr = err
			} else {
				select {
				case <-a.service.done:
					cleanupErr = a.service.exitErr
				case <-time.After(20 * time.Second):
					cleanupErr = errors.New("test controller cleanup timed out; private environment retained")
				}
			}
		}
	}
	instances, err := os.ReadDir(filepath.Join(a.profile, "state", "lima"))
	if err != nil && !os.IsNotExist(err) {
		cleanupErr = errors.Join(cleanupErr, err)
	}
	for _, instance := range instances {
		if !instance.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(a.profile, "state", "lima", instance.Name(), "lima.yaml")); err != nil {
			continue
		}
		// Partial setup owns base-building before an environment VM exists.
		if _, err := a.command(ctx, "stop", instance.Name()); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	if _, err := os.Stat(filepath.Join(a.profile, "proxy", "managed-service.plist")); err == nil {
		if _, err := a.command(ctx, "proxy-stop"); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

func until(ctx context.Context, limit time.Duration, test func() (bool, error)) error {
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	for {
		done, err := test()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("acceptance step timed out")
		case <-time.After(500 * time.Millisecond):
		}
	}
}
