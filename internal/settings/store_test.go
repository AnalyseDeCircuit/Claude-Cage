package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/privatefile"
)

func TestStoreKeepsExistingFilesAndRestorableProfile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "custom-config.json")
	store := New(dir, configPath)
	previous := Profile{Config: cage.DefaultConfig(), Preferences: Preferences{Keyboard: "mac"}}
	previous.Config.ExitPolicy = cage.ExitPolicy{Mode: "dynamic", Scope: "country"}
	next := previous
	next.Config.Timezone = "Asia/Tokyo"
	next.Preferences = Preferences{Keyboard: "native", Workspace: "/project", BrowserHomeURL: "https://example.com/", AutoTimezone: true, AutoLanguage: true}
	if err := store.Backup(previous); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(previous.Config, next); err != nil {
		t.Fatal(err)
	}
	saved, err := cage.Load(configPath)
	if err != nil || saved != next.Config {
		t.Fatal("environment config was not written to its existing path", err)
	}
	if got := store.ReadPreferences(Preferences{}); got != next.Preferences {
		t.Fatalf("preferences changed during persistence: %+v", got)
	}
	backup, err := store.Previous()
	if err != nil || backup != previous || !store.HasPrevious() {
		t.Fatalf("backup was changed by saving the next profile: %+v, %v", backup, err)
	}
	// Existing readers still see config/preferences and the original field names.
	for _, name := range []string{"custom-config.json", "workbench.json", "previous-profile.json"} {
		stat, err := os.Stat(filepath.Join(dir, name))
		if err != nil || stat.Mode().Perm() != 0600 {
			t.Fatal("settings file is not private", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "previous-profile.json"))
	var raw map[string]json.RawMessage
	if err != nil || json.Unmarshal(data, &raw) != nil || raw["config"] == nil || raw["preferences"] == nil || len(raw) != 2 {
		t.Fatal("previous-profile format changed")
	}
}

func TestStoreSaveFailureKeepsPreviousConfiguration(t *testing.T) {
	for _, failedFile := range []string{"config", "preferences"} {
		t.Run(failedFile, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "config.json")
			store := New(dir, configPath)
			previous := Profile{Config: cage.DefaultConfig(), Preferences: Preferences{Keyboard: "mac"}}
			previous.Config.ExitPolicy = cage.ExitPolicy{Mode: "dynamic", Scope: "country"}
			next := previous
			next.Config.Timezone = "Asia/Tokyo"
			next.Preferences.Keyboard = "native"
			blocked := filepath.Join(dir, "workbench.json")
			if failedFile == "config" {
				blocked = configPath
				if err := store.SavePreferences(previous.Preferences); err != nil {
					t.Fatal(err)
				}
			} else if err := privatefile.WriteJSON(configPath, previous.Config); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(blocked, 0700); err != nil {
				t.Fatal(err)
			}
			if err := store.Save(previous.Config, next); err == nil {
				t.Fatal("failed settings write was reported as successful")
			}
			if failedFile == "config" {
				if got := store.ReadPreferences(Preferences{}); got != previous.Preferences {
					t.Fatal("failed config write replaced preferences")
				}
			} else {
				cfg, err := cage.Load(configPath)
				if err != nil || cfg != previous.Config {
					t.Fatal("failed preference write did not restore the old config", err)
				}
			}
		})
	}
}

func TestStoreReadsLegacyPreferencesAndUnavailableBackups(t *testing.T) {
	dir := t.TempDir()
	store := New(dir, filepath.Join(dir, "config.json"))
	defaults := Preferences{Keyboard: "mac"}
	if got := store.ReadPreferences(defaults); got != defaults || store.HasPrevious() {
		t.Fatal("missing profile changed startup defaults or advertised a backup")
	}
	if err := os.WriteFile(filepath.Join(dir, "workbench.json"), []byte(`{"workspace":"/old/project"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := store.ReadPreferences(defaults); got.Workspace != "/old/project" || got.Keyboard != "mac" || got.HomeURL() != DefaultBrowserHomeURL {
		t.Fatal("legacy preferences lost their values or defaults")
	}
	if _, err := store.Previous(); err == nil || err.Error() != "没有可恢复的配置" {
		t.Fatal("missing backup was accepted", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "previous-profile.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Previous(); err == nil || err.Error() != "没有可恢复的配置" {
		t.Fatal("corrupt backup was accepted", err)
	}
}
