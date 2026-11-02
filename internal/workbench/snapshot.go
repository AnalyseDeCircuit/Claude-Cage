package workbench

import (
	"errors"
	"slices"
	"time"

	"github.com/sylvinus/agent-vm/internal/cage"
)

// Snapshot is the existing status contract, detached from live service state.
// Reading it does not probe guest tools or require a connected interface.
type Snapshot struct {
	Projects                  []string     `json:"projects"`
	Ready                     bool         `json:"ready"`
	VMRunning                 bool         `json:"vm_running"`
	Workspace                 string       `json:"workspace"`
	EnvironmentWorkspace      bool         `json:"environment_workspace"`
	EnvironmentDirectory      string       `json:"environment_directory"`
	ProjectError              string       `json:"project_error"`
	Keyboard                  string       `json:"keyboard"`
	AutoTimezone              bool         `json:"auto_timezone"`
	Job                       Job          `json:"job"`
	Location                  *Location    `json:"location"`
	NetworkError              string       `json:"network_error"`
	CanRestore                bool         `json:"can_restore"`
	Safety                    SafetyStatus `json:"safety"`
	EnvironmentUpdateRequired bool         `json:"environment_update_required"`
	EnvironmentUpdateError    string       `json:"environment_update_error"`
}

type SafetyStatus struct {
	Gate       *cage.GateStatus   `json:"gate,omitempty"`
	Report     *cage.SafetyReport `json:"report,omitempty"`
	Error      string             `json:"error,omitempty"`
	ExitIP     string             `json:"exit_ip,omitempty"`
	VerifiedAt time.Time          `json:"verified_at,omitempty"`
}

func (s *Service) Snapshot() (Snapshot, error) {
	projects, err := s.projectPaths()
	if err != nil {
		return Snapshot{}, errors.New("无法读取项目列表，请检查项目配置")
	}
	projectError := s.projectManager().RestoreError()
	s.mu.Lock()
	defer s.mu.Unlock()
	directory := s.environmentDir()
	status := Snapshot{
		Projects: projects, Ready: s.ready, VMRunning: s.vmRunning(),
		Workspace: s.preferences.Workspace, EnvironmentWorkspace: s.preferences.Workspace == directory,
		EnvironmentDirectory: directory, ProjectError: projectError, Keyboard: s.preferences.Keyboard,
		AutoTimezone: s.preferences.AutoTimezone, Job: s.jobQueueLocked().Snapshot(),
		NetworkError: s.networkError, CanRestore: s.settingsStore().HasPrevious(),
		Safety: SafetyStatus{
			Report: copySafetyReport(s.safety.Report), Error: s.safety.Error,
			ExitIP: s.safety.ExitIP, VerifiedAt: s.safety.VerifiedAt,
		},
		EnvironmentUpdateRequired: s.guestUpdateRequired, EnvironmentUpdateError: s.guestUpdateError,
	}
	if s.location != nil {
		location := *s.location
		status.Location = &location
	}
	if s.safety.Gate != nil {
		gate := *s.safety.Gate
		gate.Report = copySafetyReport(gate.Report)
		status.Safety.Gate = &gate
	}
	return status, nil
}

func copySafetyReport(report *cage.SafetyReport) *cage.SafetyReport {
	if report == nil {
		return nil
	}
	copy := *report
	copy.Checks = slices.Clone(report.Checks)
	return &copy
}
