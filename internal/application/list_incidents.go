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

// Occurrence-history bounds for a single incident's detail view.
const (
	defaultOccurrenceLimit = 100
	maxOccurrenceLimit     = 500
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

// Detail resolves the GitLab project id to the registered project, loads the
// incident identified by fingerprint, and returns it enriched with its
// occurrence history, a per-ref breakdown and heuristic signals. It returns
// ErrProjectNotFound when the project is not registered and ErrIncidentNotFound
// when the project has no such fingerprint.
func (s *IncidentService) Detail(ctx context.Context, gitlabProjectID int64, fingerprint string, limit int) (domain.IncidentDetail, error) {
	project, found, err := s.projects.GetByGitLabID(ctx, gitlabProjectID)
	if err != nil {
		return domain.IncidentDetail{}, err
	}
	if !found {
		return domain.IncidentDetail{}, ErrProjectNotFound
	}

	incident, found, err := s.incidents.GetByFingerprint(ctx, project.ID, fingerprint)
	if err != nil {
		return domain.IncidentDetail{}, err
	}
	if !found {
		return domain.IncidentDetail{}, ErrIncidentNotFound
	}

	occurrences, err := s.incidents.ListOccurrences(ctx, incident.FingerprintID, clampOccurrenceLimit(limit))
	if err != nil {
		return domain.IncidentDetail{}, err
	}

	return domain.AnalyzeIncident(incident, occurrences), nil
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

// clampOccurrenceLimit applies the default and ceiling for the occurrence
// history size.
func clampOccurrenceLimit(limit int) int {
	if limit <= 0 {
		return defaultOccurrenceLimit
	}
	if limit > maxOccurrenceLimit {
		return maxOccurrenceLimit
	}
	return limit
}
