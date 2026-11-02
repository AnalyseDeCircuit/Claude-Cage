package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/privatefile"
)

// Store keeps the existing configuration, preferences and previous-profile
// files. The caller serializes writes and coordinates running services.
type Store struct {
	profile, configPath string
}

func New(profile, configPath string) *Store {
	return &Store{profile: profile, configPath: configPath}
}

// ReadPreferences retains the startup defaults for missing fields, as existing
// profiles do. Read and decode failures keep the original startup behavior.
func (s *Store) ReadPreferences(defaults Preferences) Preferences {
	if b, err := os.ReadFile(filepath.Join(s.profile, "workbench.json")); err == nil {
		_ = json.Unmarshal(b, &defaults)
	}
	return defaults
}

func (s *Store) SavePreferences(p Preferences) error {
	return privatefile.WriteJSON(filepath.Join(s.profile, "workbench.json"), p)
}

// Save publishes the environment config before its preferences. If the second
// write fails, it attempts to restore the previous config in the original order.
func (s *Store) Save(previous cage.Config, next Profile) error {
	if err := privatefile.WriteJSON(s.configPath, next.Config); err != nil {
		return err
	}
	if err := s.SavePreferences(next.Preferences); err != nil {
		_ = privatefile.WriteJSON(s.configPath, previous)
		return err
	}
	return nil
}

func (s *Store) Backup(previous Profile) error {
	return privatefile.WriteJSON(filepath.Join(s.profile, "previous-profile.json"), previous)
}

func (s *Store) Previous() (Profile, error) {
	var previous Profile
	b, err := os.ReadFile(filepath.Join(s.profile, "previous-profile.json"))
	if err != nil || json.Unmarshal(b, &previous) != nil {
		return previous, errors.New("没有可恢复的配置")
	}
	return previous, nil
}

func (s *Store) HasPrevious() bool {
	_, err := os.Stat(filepath.Join(s.profile, "previous-profile.json"))
	return err == nil
}
