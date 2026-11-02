// Package settings owns settings values, validation and private profile files.
package settings

import (
	"errors"
	"net/url"
	"strings"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/projects"
)

type Preferences struct {
	Workspace      string `json:"workspace"`
	BrowserHomeURL string `json:"browser_home_url"`
	Keyboard       string `json:"keyboard"`
	AutoTimezone   bool   `json:"auto_timezone"`
	AutoLanguage   bool   `json:"auto_language"`
}

const DefaultBrowserHomeURL = "https://www.bing.com/"

func (p Preferences) HomeURL() string {
	if p.BrowserHomeURL == "" {
		return DefaultBrowserHomeURL
	}
	return p.BrowserHomeURL
}

type Values struct {
	Preferences
	DNSURL          string          `json:"dns_url"`
	Timezone        string          `json:"timezone"`
	Locale          string          `json:"locale"`
	BrowserLanguage string          `json:"browser_language"`
	ExitPolicy      cage.ExitPolicy `json:"exit_policy"`
}
type Profile struct {
	Config      cage.Config `json:"config"`
	Preferences Preferences `json:"preferences"`
}

func View(c cage.Config, p Preferences) Values {
	p.BrowserHomeURL = p.HomeURL()
	return Values{Preferences: p, DNSURL: c.DNSURL, Timezone: c.Timezone, Locale: c.Locale, BrowserLanguage: c.BrowserLanguage, ExitPolicy: c.ExitPolicy.Normalized()}
}
func Propose(input Values, old cage.Config, profile string) (cage.Config, Preferences, error) {
	if input.ExitPolicy == (cage.ExitPolicy{}) {
		input.ExitPolicy = old.ExitPolicy
	}
	c := cage.Config{ProxyURL: old.ProxyURL, ProxyRevision: old.ProxyRevision, DNSURL: input.DNSURL, Timezone: input.Timezone, Locale: input.Locale, BrowserLanguage: input.BrowserLanguage, ExitPolicy: input.ExitPolicy.Normalized()}
	if err := c.Validate(); err != nil {
		return c, input.Preferences, err
	}
	if input.Keyboard != "mac" && input.Keyboard != "native" {
		return c, input.Preferences, errors.New("请选择 Mac 或原生 Linux 按键习惯")
	}
	p := input.Preferences
	p.BrowserHomeURL = strings.TrimSpace(p.HomeURL())
	home, e := url.Parse(p.BrowserHomeURL)
	if e != nil || home == nil || (home.Scheme != "https" && home.Scheme != "http") || home.Hostname() == "" || home.User != nil {
		return c, p, errors.New("浏览器默认网站必须是完整的 HTTP 或 HTTPS 网址，且不能包含用户名或密码")
	}
	if p.Workspace != "" {
		workspace, err := projects.ValidateWorkspace(p.Workspace, profile)
		if err != nil {
			return c, p, err
		}
		p.Workspace = workspace
	}
	return c, p, nil
}

// RequiresRestart excludes live preferences such as the selected project,
// keyboard and browser home. Automatic region options still require a restart.
func (p Profile) RequiresRestart(previous Profile) bool {
	return p.Config != previous.Config || p.Preferences.AutoTimezone != previous.Preferences.AutoTimezone || p.Preferences.AutoLanguage != previous.Preferences.AutoLanguage
}
