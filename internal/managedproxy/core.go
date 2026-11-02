package managedproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// CorePath deliberately never searches PATH or another application's bundle.
func CorePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	for _, p := range []string{filepath.Join(filepath.Dir(exe), "mihomo"), filepath.Join(filepath.Dir(exe), "..", "proxy-core", "mihomo")} {
		if st, e := os.Stat(p); e == nil && st.Mode().IsRegular() && st.Mode()&0111 != 0 {
			return filepath.Clean(p), nil
		}
	}
	return "", errors.New("Cage 随附代理核心缺失，请重新安装完整应用；开发构建请运行 make proxy-core")
}
func Validate(ctx context.Context, profile string, s Store) error {
	core, err := CorePath()
	if err != nil {
		return err
	}
	b, err := s.Runtime(profile)
	if err != nil {
		return err
	}
	dir := Dir(profile)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "validate-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	f.Close()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, core, "-t", "-d", dir, "-f", f.Name())
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + dir}
	if output, e := cmd.CombinedOutput(); e != nil {
		_ = Write(filepath.Join(dir, "validation.log"), output)
		return errors.New("代理核心拒绝该配置，请检查节点协议和参数；原配置已保留")
	}
	return nil
}
func API(ctx context.Context, profile, method, path string, body any, reply any) error {
	tr := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", Socket(profile))
	}}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://cage-proxy"+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return errors.New("Cage 代理核心尚未就绪")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("代理核心操作失败（HTTP %d）", res.StatusCode)
	}
	if reply != nil {
		if json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(reply) != nil {
			return errors.New("代理核心响应无效")
		}
	}
	return nil
}
func Running(ctx context.Context, profile string, s Store) bool {
	var cfg struct {
		Port      int    `json:"mixed-port"`
		Interface string `json:"interface-name"`
	}
	var group struct {
		Now string `json:"now"`
	}
	bound, err := BoundInterface(filepath.Join(Dir(profile), "config.yaml"))
	return err == nil && bound != "" && API(ctx, profile, "GET", "/configs", nil, &cfg) == nil && cfg.Port == s.Port && cfg.Interface == bound && API(ctx, profile, "GET", "/proxies/"+Group, nil, &group) == nil && group.Now == s.Selected
}
func servicePath(profile string) string { return filepath.Join(Dir(profile), "managed-service.plist") }
