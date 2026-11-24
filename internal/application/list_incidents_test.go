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

	// detail-view fields
	header           domain.Incident
	headerFound      bool
	occurrences      []domain.IncidentOccurrence
	gotFingerprint   string
	gotOccurrenceID  string
	gotOccurrenceLim int
}

func (r *fakeIncidentRepo) ListByProject(_ context.Context, projectID domain.ProjectID, limit int) ([]domain.Incident, error) {
	r.gotProject = projectID
	r.gotLimit = limit
	return r.incidents, nil
}

func (r *fakeIncidentRepo) GetByFingerprint(_ context.Context, projectID domain.ProjectID, fingerprint string) (domain.Incident, bool, error) {
	r.gotProject = projectID
	r.gotFingerprint = fingerprint
	return r.header, r.headerFound, nil
}

func (r *fakeIncidentRepo) ListOccurrences(_ context.Context, fingerprintID string, limit int) ([]domain.IncidentOccurrence, error) {
	r.gotOccurrenceID = fingerprintID
	r.gotOccurrenceLim = limit
	return r.occurrences, nil
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

func TestIncidentService_Detail(t *testing.T) {
	ctx := context.Background()
	projects := newFakeProjectRepo()
	stored := registerProjectForTest(t, projects, 1001)

	incidents := &fakeIncidentRepo{
		header:      domain.Incident{FingerprintID: "fp-id", Fingerprint: "abc", Category: "test_failure", OccurrenceCount: 3},
		headerFound: true,
		occurrences: []domain.IncidentOccurrence{
			{Ref: "main", SHA: "aaa"},
			{Ref: "main", SHA: "aaa"},
			{Ref: "feature", SHA: "bbb"},
		},
	}
	svc := NewIncidentService(projects, incidents)

	got, err := svc.Detail(ctx, 1001, "abc", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Fingerprint != "abc" || len(got.Occurrences) != 3 {
		t.Errorf("detail = %+v, want fingerprint abc with 3 occurrences", got)
	}
	// Signals are computed from the occurrence history: same commit twice ->
	// flaky; two distinct commits -> regression.
	if !got.Signals.FlakySuspected || !got.Signals.RegressionSuspected {
		t.Errorf("signals = %+v, want both flaky and regression suspected", got.Signals)
	}
	if incidents.gotProject != stored.ID {
		t.Errorf("queried project = %q, want resolved id %q", incidents.gotProject, stored.ID)
	}
	if incidents.gotFingerprint != "abc" {
		t.Errorf("queried fingerprint = %q, want abc", incidents.gotFingerprint)
	}
	if incidents.gotOccurrenceID != "fp-id" {
		t.Errorf("occurrence query id = %q, want the header's FingerprintID fp-id", incidents.gotOccurrenceID)
	}
	if incidents.gotOccurrenceLim != defaultOccurrenceLimit {
		t.Errorf("occurrence limit = %d, want default %d", incidents.gotOccurrenceLim, defaultOccurrenceLimit)
	}
}

func TestIncidentService_Detail_UnknownProject(t *testing.T) {
	projects := newFakeProjectRepo()
	svc := NewIncidentService(projects, &fakeIncidentRepo{})

	_, err := svc.Detail(context.Background(), 9999, "abc", 0)
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("err = %v, want ErrProjectNotFound", err)
	}
}

func TestIncidentService_Detail_UnknownFingerprint(t *testing.T) {
	projects := newFakeProjectRepo()
	registerProjectForTest(t, projects, 1001)
	// headerFound defaults to false -> incident not found.
	svc := NewIncidentService(projects, &fakeIncidentRepo{})

	_, err := svc.Detail(context.Background(), 1001, "missing", 0)
	if !errors.Is(err, ErrIncidentNotFound) {
		t.Fatalf("err = %v, want ErrIncidentNotFound", err)
	}
}

func TestIncidentService_Detail_ClampsOccurrenceLimit(t *testing.T) {
	projects := newFakeProjectRepo()
	registerProjectForTest(t, projects, 1001)
	incidents := &fakeIncidentRepo{header: domain.Incident{FingerprintID: "fp-id"}, headerFound: true}
	svc := NewIncidentService(projects, incidents)

	if _, err := svc.Detail(context.Background(), 1001, "abc", 9000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if incidents.gotOccurrenceLim != maxOccurrenceLimit {
		t.Errorf("occurrence limit = %d, want clamped to %d", incidents.gotOccurrenceLim, maxOccurrenceLimit)
	}
}
