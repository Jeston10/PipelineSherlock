package application

import (
	"context"
	"errors"
	"testing"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// fakeIncidentRepo returns canned incidents and records how it was called.
type fakeIncidentRepo struct {
	incidents  []domain.Incident
	gotProject domain.ProjectID
	gotLimit   int
}

func (r *fakeIncidentRepo) ListByProject(_ context.Context, projectID domain.ProjectID, limit int) ([]domain.Incident, error) {
	r.gotProject = projectID
	r.gotLimit = limit
	return r.incidents, nil
}

func registerProjectForTest(t *testing.T, projects *fakeProjectRepo, gitlabID int64) domain.Project {
	t.Helper()
	p, err := domain.NewProject(gitlabID, "svc", "team")
	if err != nil {
		t.Fatalf("new project: %v", err)
	}
	stored, err := projects.Create(context.Background(), p)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return stored
}

func TestIncidentService_List(t *testing.T) {
	ctx := context.Background()
	projects := newFakeProjectRepo()
	stored := registerProjectForTest(t, projects, 1001)

	incidents := &fakeIncidentRepo{incidents: []domain.Incident{
		{FingerprintID: "fp1", Category: "test_failure", OccurrenceCount: 3},
	}}
	svc := NewIncidentService(projects, incidents)

	got, err := svc.List(ctx, 1001, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].FingerprintID != "fp1" {
		t.Errorf("incidents = %+v, want one fp1", got)
	}
	if incidents.gotProject != stored.ID {
		t.Errorf("queried project = %q, want the resolved internal id %q", incidents.gotProject, stored.ID)
	}
	if incidents.gotLimit != defaultIncidentLimit {
		t.Errorf("limit = %d, want default %d", incidents.gotLimit, defaultIncidentLimit)
	}
}

func TestIncidentService_UnknownProject(t *testing.T) {
	projects := newFakeProjectRepo()
	svc := NewIncidentService(projects, &fakeIncidentRepo{})

	_, err := svc.List(context.Background(), 9999, 10)
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("err = %v, want ErrProjectNotFound", err)
	}
}

func TestIncidentService_ClampsLimit(t *testing.T) {
	projects := newFakeProjectRepo()
	registerProjectForTest(t, projects, 1001)
	incidents := &fakeIncidentRepo{}
	svc := NewIncidentService(projects, incidents)

	if _, err := svc.List(context.Background(), 1001, 500); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if incidents.gotLimit != maxIncidentLimit {
		t.Errorf("limit = %d, want clamped to %d", incidents.gotLimit, maxIncidentLimit)
	}
}
