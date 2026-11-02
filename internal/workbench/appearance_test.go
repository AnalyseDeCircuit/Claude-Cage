package workbench

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCodexAppearanceImportsOnlyAppearance(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	data := `model = "unrelated"
[desktop]
sansFontSize = 16
codeFontSize = 13
appearanceLightCodeThemeId = "solarized"
appearanceDarkCodeThemeId = "ayu"
[desktop.appearanceLightChromeTheme]
accent = "#b58900"
surface = "#fdf6e3"
ink = "#657b83"
contrast = 45
opaqueWindows = false
[desktop.appearanceDarkChromeTheme]
accent = "#e6b450"
surface = "#10141c"
ink = "#bfbdb6"
contrast = 60
opaqueWindows = true
[desktop.appearanceDarkChromeTheme.fonts]
code = "Menlo"
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".codex-global-state.json"), []byte(`{"reduced-motion-preference":"on","irrelevant":"secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := readCodexAppearance(path)
	if err != nil {
		t.Fatal(err)
	}
	if a.Mode != "system" || a.Light.Preset != "solarized" || a.Dark.Surface != "#10141c" || a.UIFontSize != 16 || a.CodeFontSize != 13 || a.Dark.Fonts.Code != "Menlo" || a.ReducedMotion != "on" || !a.FontSmoothing {
		t.Fatalf("unexpected appearance: %+v", a)
	}
	b, _ := json.Marshal(a)
	if bytes.Contains(b, []byte("secret")) || bytes.Contains(b, []byte("unrelated")) {
		t.Fatal("non-appearance data leaked")
	}
}

func TestAppearanceSaveDoesNotChangeEnvironmentAndRejectsBadInput(t *testing.T) {
	s := &Server{Service: &Service{profile: t.TempDir(), preferences: Preferences{Workspace: "/project"}}}
	holdTestJob(t, s, "start")
	a := defaultAppearance()
	a.Mode = "dark"
	a.UIFontSize = 16
	b, _ := json.Marshal(a)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/appearance", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	s.saveAppearance(w, req)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	job := s.jobQueue().Snapshot()
	if !job.Running || job.Kind != "start" || s.preferences.Workspace != "/project" {
		t.Fatal("appearance changed environment state")
	}
	w = httptest.NewRecorder()
	s.getAppearance(w, httptest.NewRequest("GET", "/api/appearance", nil))
	var saved Appearance
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil || saved != a {
		t.Fatalf("appearance did not round trip: %s", w.Body.String())
	}
	a.UIFontSize = 400
	b, _ = json.Marshal(a)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/appearance", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	s.saveAppearance(w, req)
	if w.Code == 200 {
		t.Fatal("invalid appearance accepted")
	}
	stored, _ := os.ReadFile(filepath.Join(s.profile, "appearance.json"))
	if bytes.Contains(stored, []byte("400")) {
		t.Fatal("invalid settings replaced saved appearance")
	}
}
