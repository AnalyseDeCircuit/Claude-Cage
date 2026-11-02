package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sylvinus/agent-vm/internal/cage"
)

func TestSettingsPreserveSecretsAndRejectHostSharing(t *testing.T) {
	profile := t.TempDir()
	old := cage.DefaultConfig()
	old.ProxyURL = "socks5://user:secret@127.0.0.1:17896"
	old.ProxyRevision = "current-revision"
	input := View(old, Preferences{Keyboard: "mac"})
	input.Keyboard = "native"
	next, _, e := Propose(input, old, profile)
	if e != nil {
		t.Fatal(e)
	}
	if next.ProxyURL != old.ProxyURL || next.ProxyRevision != old.ProxyRevision {
		t.Fatal("blank password erased existing authentication")
	}
	input.Workspace = profile
	if _, _, e = Propose(input, old, profile); e == nil {
		t.Fatal("profile may be shared")
	}
	input.Workspace = filepath.Dir(profile)
	if _, _, e = Propose(input, old, profile); e == nil {
		t.Fatal("profile ancestor may be shared")
	}
	workspace := t.TempDir()
	link := filepath.Join(workspace, "private-link")
	if e = os.Symlink(profile, link); e != nil {
		t.Fatal(e)
	}
	input.Workspace = link
	if _, _, e = Propose(input, old, profile); e == nil {
		t.Fatal("symlink bypassed share guard")
	}
}
func TestBrowserHomeValidationAndDefault(t *testing.T) {
	profile := t.TempDir()
	cfg := cage.DefaultConfig()
	input := View(cfg, Preferences{Keyboard: "mac"})
	if input.BrowserHomeURL != DefaultBrowserHomeURL {
		t.Fatal("legacy preferences did not default to Bing")
	}
	for _, value := range []string{"chrome://new-tab-page-third-party/", "javascript:alert(1)", "file:///private", "https://user:secret@example.com/", "https://", "bing.com"} {
		input.BrowserHomeURL = value
		if _, _, err := Propose(input, cfg, profile); err == nil {
			t.Fatalf("accepted invalid home %q", value)
		}
	}
	input.BrowserHomeURL = " https://example.com/start?q=hello "
	_, p, err := Propose(input, cfg, profile)
	if err != nil || p.BrowserHomeURL != "https://example.com/start?q=hello" {
		t.Fatalf("valid home: %v", err)
	}
}

func TestMissingExitPolicyDoesNotRelaxSavedStaticRule(t *testing.T) {
	profile := t.TempDir()
	old := cage.DefaultConfig()
	old.ExitPolicy = cage.ExitPolicy{Mode: "static", Scope: "country", IP: "8.8.8.8"}
	input := View(old, Preferences{Keyboard: "mac"})
	input.ExitPolicy = cage.ExitPolicy{}
	got, _, err := Propose(input, old, profile)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExitPolicy != old.ExitPolicy {
		t.Fatal("legacy form silently relaxed static IP")
	}
}

func TestProfileRestartScope(t *testing.T) {
	previous := Profile{Config: cage.DefaultConfig(), Preferences: Preferences{Keyboard: "mac"}}
	for _, tc := range []struct {
		name    string
		change  func(*Profile)
		restart bool
	}{
		{"unchanged", func(*Profile) {}, false},
		{"project", func(p *Profile) { p.Preferences.Workspace = "/another/project" }, false},
		{"keyboard", func(p *Profile) { p.Preferences.Keyboard = "native" }, false},
		{"homepage", func(p *Profile) { p.Preferences.BrowserHomeURL = "https://example.com/" }, false},
		{"timezone", func(p *Profile) { p.Config.Timezone = "Asia/Tokyo" }, true},
		{"auto-timezone", func(p *Profile) { p.Preferences.AutoTimezone = true }, true},
		{"auto-language", func(p *Profile) { p.Preferences.AutoLanguage = true }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := previous
			tc.change(&next)
			if got := next.RequiresRestart(previous); got != tc.restart {
				t.Fatalf("restart = %v, want %v", got, tc.restart)
			}
		})
	}
}
