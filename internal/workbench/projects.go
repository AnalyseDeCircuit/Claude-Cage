package workbench

import (
	"context"
	"os/exec"

	"github.com/sylvinus/agent-vm/internal/projects"
	"github.com/sylvinus/agent-vm/internal/vmname"
)

// The workbench supplies its existing authenticated connection; projects owns
// directory grants and share processes, without depending on HTTP or UI state.
type projectGuest struct{ service *Service }

func (g projectGuest) Run(ctx context.Context, script string) ([]byte, error) {
	return g.service.guest(ctx, script)
}

func (g projectGuest) Command(ctx context.Context, script string) *exec.Cmd {
	s := g.service
	return s.sshCommand(ctx, "-T", "lima-"+vmname.Name(s.environmentDir()), "--", script)
}

func (s *Service) projectManager() *projects.Manager {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.projects == nil {
		s.projects = projects.New(s.profile, s.environmentDir(), projectGuest{s})
	}
	return s.projects
}

func (s *Service) ensureProjectShare(path string) error {
	return s.projectManager().EnsureShare(s.operationContext(), path)
}

func (s *Service) projectPaths() ([]string, error) {
	return s.projectManager().Paths(s.workspace())
}

func (s *Service) projectWorkspace(path string) (string, error) {
	return s.projectManager().Workspace(path, s.workspace())
}

func (s *Service) rememberSharedProject(path string) error {
	return s.projectManager().Remember(path)
}

func (s *Service) restoreProjectShares() {
	s.projectManager().Restore(s.operationContext(), s.workspace())
}

func (s *Service) closeProjectShares() {
	s.projectManager().Close()
}
