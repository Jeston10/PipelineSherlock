package application

import (
	"context"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// Incident listing bounds.
const (
	defaultIncidentLimit = 50
	maxIncidentLimit     = 200
)

// IncidentService is the read-side use case for grouped failure incidents.
type IncidentService struct {
	projects  ProjectRepository
	incidents IncidentRepository
}

// NewIncidentService wires the service.
func NewIncidentService(projects ProjectRepository, incidents IncidentRepository) *IncidentService {
	return &IncidentService{projects: projects, incidents: incidents}
}

// List resolves the GitLab project id to the registered project and returns its
// incidents, most recently seen first. It returns ErrProjectNotFound when the
// project is not registered.
func (s *IncidentService) List(ctx context.Context, gitlabProjectID int64, limit int) ([]domain.Incident, error) {
	project, found, err := s.projects.GetByGitLabID(ctx, gitlabProjectID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrProjectNotFound
	}
	return s.incidents.ListByProject(ctx, project.ID, clampIncidentLimit(limit))
}

// clampIncidentLimit applies the default and ceiling for the list size.
func clampIncidentLimit(limit int) int {
	if limit <= 0 {
		return defaultIncidentLimit
	}
	if limit > maxIncidentLimit {
		return maxIncidentLimit
	}
	return limit
}
