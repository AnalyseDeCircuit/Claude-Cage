// Package browser connects to the guest browser controller without owning UI,
// environment readiness or project directory authorization.
package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

var (
	ErrUnknownAction = errors.New("未知浏览器操作")
	ErrInvalidText   = errors.New("输入文字或目标标签无效")
)

// Input is the existing guest command payload. Workspace must already be
// authorized and shared by the caller before opening or restarting an editor.
type Input struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Text      string `json:"text,omitempty"`
}

type Client struct {
	address  string
	commands *http.Client
}

// New connects to a controller address supplied by the host, never a tab URL.
// Requests bypass environment proxies and refuse redirects. Credentials are
// supplied per operation so reconnecting does not retain an old guest token.
func New(address string) *Client {
	transport := newTransport()
	transport.IdleConnTimeout = 30 * time.Second
	transport.MaxIdleConnsPerHost = 4
	return &Client{address: address, commands: &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: refuseRedirect}}
}

func (c *Client) CloseIdleConnections() { c.commands.CloseIdleConnections() }

func newTransport() *http.Transport {
	return &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext}
}

func refuseRedirect(*http.Request, []*http.Request) error { return errors.New("redirect refused") }

func Supports(action string) bool {
	switch action {
	case "tabs", "new", "activate", "close", "navigate", "back", "forward", "reload", "editor", "editor-restart", "download-retry", "resize", "insert-text":
		return true
	}
	return false
}

func (c *Client) Tabs(ctx context.Context, token string) ([]byte, error) {
	return c.request(ctx, token, "GET", "/tabs", nil)
}

func (c *Client) Command(ctx context.Context, token, action string, input Input) ([]byte, error) {
	if !Supports(action) {
		return nil, ErrUnknownAction
	}
	if action == "insert-text" && (input.ID == "" || input.Text == "" || len(input.Text) > 8192) {
		return nil, ErrInvalidText
	}
	body, _ := json.Marshal(input)
	return c.request(ctx, token, "POST", "/"+action, body)
}

func (c *Client) Health(ctx context.Context, token string) error {
	_, err := c.request(ctx, token, "GET", "/health", nil)
	return err
}

func (c *Client) SetHome(ctx context.Context, token, home string) error {
	body, _ := json.Marshal(map[string]string{"url": home})
	_, err := c.request(ctx, token, "POST", "/homepage", body)
	return err
}

func (c *Client) Audit(ctx context.Context, token string, network bool, timezone string) ([]byte, error) {
	body, _ := json.Marshal(map[string]any{"network": network, "timezone": timezone})
	return c.request(ctx, token, "POST", "/audit", body)
}

func (c *Client) request(ctx context.Context, token, method, path string, body []byte) ([]byte, error) {
	client := *c.commands
	switch path {
	case "/audit":
		client.Timeout = 20 * time.Second
	case "/editor-restart":
		client.Timeout = 30 * time.Second
	}
	req, err := http.NewRequestWithContext(ctx, method, c.address+path, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("浏览器服务暂时无法连接")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.New("浏览器服务暂时无法连接")
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, errors.New("浏览器响应中断")
	}
	if res.StatusCode != 200 {
		return nil, errors.New("浏览器操作失败，请刷新标签后重试")
	}
	return b, nil
}
