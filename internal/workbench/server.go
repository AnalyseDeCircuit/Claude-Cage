package workbench

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed web/*
var assets embed.FS

const cookieName = "cage_session"

type controlFile struct {
	Port       int                `json:"port"`
	Token      string             `json:"token"`
	PID        int                `json:"pid"`
	Controller ControllerIdentity `json:"controller"`
}
type Server struct {
	*Service
	token, session string
	tickets        map[string]time.Time
	shutdown       func()
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func equalSecret(a, b string) bool {
	return len(a) > 0 && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func Serve(configPath string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return ServeContext(ctx, configPath)
}

// ServeContext hosts the control service until cancellation or an authenticated
// idle handoff. A UI disconnect is unrelated to this host lifetime.
func ServeContext(ctx context.Context, configPath string) (returned error) {
	var listener net.Listener
	var err error
	port := 17777
	for ; port < 17808; port++ {
		listener, err = net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}
	defer listener.Close()
	service, err := OpenService(configPath, fmt.Sprintf("http://localhost:%d", port))
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, service.Close()) }()
	s := &Server{Service: service, token: randomToken(), session: randomToken()}
	if err := writePrivate(filepath.Join(s.profile, "control.json"), controlFile{Port: port, Token: s.token, PID: os.Getpid(), Controller: s.identity}); err != nil {
		return err
	}
	s.controlPublished = true
	server := &http.Server{Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	var shutdown sync.Once
	s.shutdown = func() {
		shutdown.Do(func() {
			s.beginClose()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
			_ = server.Close()
		})
	}
	watchDone, watchExited := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(watchExited)
		select {
		case <-ctx.Done():
			s.shutdown()
		case <-watchDone:
		}
	}()
	defer func() { close(watchDone); <-watchExited }()
	err = server.Serve(listener)
	s.shutdown()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { jsonReply(w, map[string]bool{"ready": true}) })
	mux.HandleFunc("POST /api/launch", func(w http.ResponseWriter, r *http.Request) {
		if !s.launcherAuthenticated(w, r) {
			return
		}
		jsonReply(w, launchResponse{URL: s.origin + "/?launch=" + randomToken()[:8] + "#" + s.issueTicket(), Controller: s.identity})
	})
	mux.HandleFunc("POST /api/handoff", s.handoff)
	mux.HandleFunc("POST /api/session", func(w http.ResponseWriter, r *http.Request) {
		var v struct {
			Token string `json:"token"`
		}
		if !readJSON(w, r, &v) {
			return
		}
		if !s.consumeTicket(v.Token) {
			http.Error(w, "访问凭证无效，请从 Cage 应用重新打开。", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: s.session, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 86400 * 30})
		jsonReply(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("GET /api/environment/versions", s.environmentVersions)
	mux.HandleFunc("POST /api/environment/update", s.updateEnvironment)
	mux.HandleFunc("POST /api/environment/tools", s.updateEnvironmentTools)
	mux.HandleFunc("GET /api/host/git", s.hostGit)
	mux.HandleFunc("POST /api/host/git", s.enableHostGit)
	mux.HandleFunc("GET /api/appearance", s.getAppearance)
	mux.HandleFunc("GET /api/favorites", s.getFavorites)
	mux.HandleFunc("POST /api/favorites", s.mutateFavorites)
	mux.HandleFunc("POST /api/appearance", s.saveAppearance)
	mux.HandleFunc("GET /api/appearance/codex", s.codexAppearance)
	mux.HandleFunc("POST /api/settings/probe", s.probe)
	mux.HandleFunc("GET /api/proxy", s.proxyStatus)
	mux.HandleFunc("POST /api/proxy/delay", s.proxyDelay)
	mux.HandleFunc("GET /api/proxy/connections", s.proxyConnections)
	mux.HandleFunc("GET /api/proxy/logs", s.proxyLogs)
	mux.HandleFunc("POST /api/proxy/{action}", s.proxyMutation)
	mux.HandleFunc("POST /api/settings", s.saveSettings)
	mux.HandleFunc("POST /api/restore", s.restore)
	mux.HandleFunc("POST /api/start", s.start)
	mux.HandleFunc("POST /api/stop", s.stop)
	mux.HandleFunc("POST /api/safety/check", s.safetyCheck)
	mux.HandleFunc("POST /api/safety/close", s.pauseNetwork)
	mux.HandleFunc("GET /api/safety/report", s.safetyExport)
	mux.HandleFunc("GET /api/browser/tabs", s.browser)
	mux.HandleFunc("GET /api/browser/events", s.browserEvents)
	mux.HandleFunc("POST /api/browser/{action}", s.browser)
	mux.HandleFunc("GET /api/vnc-credentials", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.ready {
			http.Error(w, "环境尚未就绪", 409)
			return
		}
		jsonReply(w, map[string]string{"password": s.credentials.VNCPassword})
	})
	mux.HandleFunc("GET /api/vnc", s.vnc)
	mux.HandleFunc("GET /api/terminals", s.listTerminals)
	mux.HandleFunc("POST /api/terminals", s.newTerminal)
	mux.HandleFunc("DELETE /api/terminals/{id}", s.closeTerminal)
	mux.HandleFunc("GET /api/terminals/{id}/stream", s.terminalStream)
	web, _ := fs.Sub(assets, "web")
	mux.Handle("GET /", http.FileServer(http.FS(web)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin, err := url.Parse(s.origin)
		if err != nil || r.Host != origin.Host {
			http.Error(w, "Host refused", 403)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self' ws://"+origin.Host+"; img-src 'self' data:; font-src 'self'; object-src 'none'; frame-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		if (r.Method != "GET" && r.Method != "HEAD") || strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			if r.Header.Get("Origin") != s.origin {
				http.Error(w, "Origin refused", 403)
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/session" && r.URL.Path != "/api/launch" && r.URL.Path != "/api/handoff" {
			c, e := r.Cookie(s.cookieName())
			if e != nil || !equalSecret(c.Value, s.session) {
				http.Error(w, "请从 Cage 应用重新打开工作台", 401)
				return
			}
		}
		ctx, done, err := s.beginActivity(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		defer done()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}
func readJSON(w http.ResponseWriter, r *http.Request, v any, limits ...int64) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "需要 JSON 请求", 415)
		return false
	}
	limit := int64(64 << 10)
	if len(limits) > 0 {
		limit = limits[0]
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		http.Error(w, "请求格式无效", 400)
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		http.Error(w, "请求格式无效", 400)
		return false
	}
	return true
}
func jsonReply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
func requestError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusBadRequest)
}
func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.Snapshot()
	if err != nil {
		requestError(w, err)
		return
	}
	jsonReply(w, snapshot)
}
func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	job, err := s.Start()
	jobReply(w, job, err)
}
func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	job, err := s.Stop()
	jobReply(w, job, err)
}

func (s *Server) cookieName() string { u, _ := url.Parse(s.origin); return cookieName + "_" + u.Port() }

func (s *Server) issueTicket() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tickets == nil {
		s.tickets = map[string]time.Time{}
	}
	for token, expires := range s.tickets {
		if time.Now().After(expires) {
			delete(s.tickets, token)
		}
	}
	token := randomToken()
	s.tickets[token] = time.Now().Add(time.Minute)
	return token
}
func (s *Server) consumeTicket(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	expires, ok := s.tickets[token]
	delete(s.tickets, token)
	return ok && time.Now().Before(expires)
}
