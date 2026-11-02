package projects

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/sylvinus/agent-vm/internal/privatefile"
)

type sharedProjects struct {
	Paths []string `json:"paths"`
}

// The saved directory grants are the project list. Include the default and
// current directories for profiles created before projects.json existed.
func (m *Manager) Paths(current string) ([]string, error) {
	var saved sharedProjects
	if b, err := os.ReadFile(filepath.Join(m.profile, "projects.json")); err == nil {
		if err = json.Unmarshal(b, &saved); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	paths := []string{m.environment}
	for _, path := range append(saved.Paths, current) {
		found := path == ""
		for _, existing := range paths {
			found = found || path == existing
		}
		if !found {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func (m *Manager) Workspace(path, current string) (string, error) {
	if path == "" {
		path = current
	}
	paths, err := m.Paths(current)
	if err != nil {
		return "", errors.New("无法读取项目列表，请检查项目配置")
	}
	for _, authorized := range paths {
		if path == authorized {
			return ValidateWorkspace(path, m.profile)
		}
	}
	return "", errors.New("请先通过打开项目添加该文件夹")
}

func (m *Manager) remember(path string) error {
	if path == m.environment {
		return nil
	}
	var saved sharedProjects
	if b, err := os.ReadFile(filepath.Join(m.profile, "projects.json")); err == nil {
		if err = json.Unmarshal(b, &saved); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for _, p := range saved.Paths {
		if p == path {
			return nil
		}
	}
	saved.Paths = append(saved.Paths, path)
	return privatefile.WriteJSON(filepath.Join(m.profile, "projects.json"), saved)
}
