package managedproxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

func label(profile string) string {
	h := sha256.Sum256([]byte(profile))
	return fmt.Sprintf("local.claude-cage.managed-proxy.%x", h[:8])
}

// The proxy is owned by the user's launchd domain, independently of windows.
func Stop(profile string) error {
	if _, err := os.Stat(servicePath(profile)); os.IsNotExist(err) {
		return nil
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	if exec.Command("launchctl", "print", domain+"/"+label(profile)).Run() != nil {
		return nil
	}
	if exec.Command("launchctl", "bootout", domain, servicePath(profile)).Run() != nil {
		return errors.New("未能停止 Cage 专属代理进程")
	}
	return nil
}
func Start(ctx context.Context, profile string, s Store) error {
	name, err := PhysicalInterface(ctx)
	if err != nil {
		return err
	}
	path := filepath.Join(Dir(profile), "config.yaml")
	bound, _ := BoundInterface(path)
	if bound == name && Running(ctx, profile, s) {
		return nil
	}
	if err := Validate(ctx, profile, s); err != nil {
		return err
	}
	if err := Stop(profile); err != nil {
		return err
	}
	// Never attach to or kill an unknown listener, even if it uses our port.
	l, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", s.Port))
	if err != nil {
		return errors.New("Cage 代理端口已被占用，请更换独立端口")
	}
	l.Close()
	core, err := CorePath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(Socket(profile)), 0700); err != nil {
		return err
	}
	if err = os.Chmod(filepath.Dir(Socket(profile)), 0700); err != nil {
		return err
	}
	b, err := s.Runtime(profile)
	if err != nil {
		return err
	}
	b, err = BindInterface(b, name)
	if err != nil {
		return err
	}
	if err = Write(path, b); err != nil {
		return err
	}
	_ = os.Remove(Socket(profile))
	log := filepath.Join(Dir(profile), "runtime.log")
	if err = Write(log, nil); err != nil {
		return err
	}
	escape := func(s string) string { var b bytes.Buffer; _ = xml.EscapeText(&b, []byte(s)); return b.String() }
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Label</key><string>%s</string><key>ProgramArguments</key><array><string>%s</string><string>-d</string><string>%s</string><string>-f</string><string>%s</string></array><key>EnvironmentVariables</key><dict><key>HOME</key><string>%s</string><key>PATH</key><string>/usr/bin:/bin</string></dict><key>RunAtLoad</key><true/><key>StandardOutPath</key><string>%s</string><key>StandardErrorPath</key><string>%s</string></dict></plist>`, escape(label(profile)), escape(core), escape(Dir(profile)), escape(filepath.Join(Dir(profile), "config.yaml")), escape(Dir(profile)), escape(log), escape(log))
	if err = Write(servicePath(profile), []byte(plist)); err != nil {
		return err
	}
	if exec.CommandContext(ctx, "launchctl", "bootstrap", "gui/"+strconv.Itoa(os.Getuid()), servicePath(profile)).Run() != nil {
		return errors.New("无法启动 Cage 自带代理核心")
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = Stop(profile)
			return ctx.Err()
		case <-deadline.C:
			_ = Stop(profile)
			return errors.New("代理核心启动超时，请查看私有代理日志")
		case <-ticker.C:
			if Running(ctx, profile, s) {
				if err = os.Chmod(Socket(profile), 0600); err != nil {
					_ = Stop(profile)
					return err
				}
				return nil
			}
		}
	}
}
