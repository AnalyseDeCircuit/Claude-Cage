package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
)

type Location struct {
	IP                       string    `json:"ip"`
	Country                  string    `json:"country"`
	CountryCode              string    `json:"country_code"`
	Region                   string    `json:"region"`
	RegionCode               string    `json:"region_code,omitempty"`
	SuggestedLocale          string    `json:"suggested_locale,omitempty"`
	SuggestedBrowserLanguage string    `json:"suggested_browser_language,omitempty"`
	City                     string    `json:"city"`
	Timezone                 string    `json:"timezone"`
	CheckedAt                time.Time `json:"checked_at"`
}

// probeLocation uses the same fail-closed transport as the VM. No host resolver,
// environment proxy, or direct fallback participates, including redirects.
func probeLocation(ctx context.Context, c cage.Config) (Location, error) {
	ip, err := cage.ProbeExitIP(ctx, c)
	if err != nil {
		return Location{}, err
	}
	loc, err := probeLocationForIP(ctx, c, ip)
	if err != nil {
		return Location{IP: ip, CheckedAt: time.Now()}, err
	}
	return loc, nil
}

func (l Location) exitLocation() cage.ExitLocation {
	return cage.ExitLocation{IP: l.IP, CountryCode: l.CountryCode, Region: l.Region, City: l.City}
}

func probeLocationForIP(ctx context.Context, c cage.Config, ip string) (Location, error) {
	address := net.ParseIP(ip)
	if address == nil {
		return Location{}, errors.New("出口地址无效")
	}
	e, err := cage.NewEgress(c)
	if err != nil {
		return Location{}, err
	}
	transport := &http.Transport{Proxy: nil, DialContext: e.DialContext, ForceAttemptHTTP2: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://ipwho.is/"+address.String()+"?fields=success,ip,country,country_code,region,region_code,city,timezone.id", nil)
	r, err := client.Do(req)
	if err != nil {
		return Location{}, errors.New("无法经指定代理查询出口；请检查代理是否运行。没有使用直连回退。")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return Location{}, fmt.Errorf("出口定位服务返回 HTTP %d；原配置保持不变", r.StatusCode)
	}
	var v struct {
		Success     bool   `json:"success"`
		IP          string `json:"ip"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
		Region      string `json:"region"`
		RegionCode  string `json:"region_code"`
		City        string `json:"city"`
		Timezone    struct {
			ID string `json:"id"`
		} `json:"timezone"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 128<<10)).Decode(&v) != nil || !v.Success || net.ParseIP(v.IP) == nil || !net.ParseIP(v.IP).Equal(address) {
		return Location{}, errors.New("出口定位服务未返回有效位置；可关闭自动时区后手动设置")
	}
	if _, err = time.LoadLocation(v.Timezone.ID); err != nil {
		return Location{}, errors.New("出口位置未包含有效时区；请手动选择时区")
	}
	locale, browser := languageSuggestion(v.CountryCode)
	return Location{IP: v.IP, Country: v.Country, CountryCode: v.CountryCode, Region: v.Region, RegionCode: v.RegionCode, City: v.City, Timezone: v.Timezone.ID, CheckedAt: time.Now(), SuggestedLocale: locale, SuggestedBrowserLanguage: browser}, nil
}
