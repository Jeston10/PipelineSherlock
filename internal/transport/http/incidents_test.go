package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Jeston10/pipeline-sherlock/internal/application"
	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

type fakeIncidentLister struct {
	incidents   []domain.Incident
	err         error
	gotGitlabID int64
	gotLimit    int

	// detail-view fields
	detail            domain.IncidentDetail
	detailErr         error
	gotFingerprint    string
	gotDetailGitlabID int64
	gotDetailLimit    int
}

func (f *fakeIncidentLister) List(_ context.Context, _ string, gitlabProjectID int64, limit int) ([]domain.Incident, error) {
	f.gotGitlabID = gitlabProjectID
	f.gotLimit = limit
	return f.incidents, f.err
}

func (f *fakeIncidentLister) Detail(_ context.Context, _ string, gitlabProjectID int64, fingerprint string, limit int) (domain.IncidentDetail, error) {
	f.gotDetailGitlabID = gitlabProjectID
	f.gotFingerprint = fingerprint
	f.gotDetailLimit = limit
	return f.detail, f.detailErr
}

func TestListIncidentsEndpoint(t *testing.T) {
	lister := &fakeIncidentLister{incidents: []domain.Incident{{
		Fingerprint:     "fp1",
		Category:        "test_failure",
		OccurrenceCount: 4,
		FirstSeenAt:     time.Unix(1000, 0).UTC(),
		LastSeenAt:      time.Unix(2000, 0).UTC(),
	}}}
	router := NewRouter(Deps{Readiness: fakeChecker{}, Incidents: lister, APIKeys: stubAuthenticator{}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/1001/incidents?limit=10", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body []incidentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 || body[0].Fingerprint != "fp1" || body[0].OccurrenceCount != 4 {
		t.Errorf("body = %+v, want one incident fp1 count 4", body)
	}
	if lister.gotGitlabID != 1001 || lister.gotLimit != 10 {
		t.Errorf("passed (gitlabID=%d, limit=%d), want (1001, 10)", lister.gotGitlabID, lister.gotLimit)
	}
}

func TestListIncidents_UnknownProject(t *testing.T) {
	lister := &fakeIncidentLister{err: application.ErrProjectNotFound}
	router := NewRouter(Deps{Readiness: fakeChecker{}, Incidents: lister, APIKeys: stubAuthenticator{}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/9999/incidents", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestListIncidents_InvalidID(t *testing.T) {
	router := NewRouter(Deps{Readiness: fakeChecker{}, Incidents: &fakeIncidentLister{}, APIKeys: stubAuthenticator{}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/not-a-number/incidents", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetIncidentEndpoint(t *testing.T) {
	lister := &fakeIncidentLister{detail: domain.IncidentDetail{
		Incident: domain.Incident{Fingerprint: "abc", Category: "test_failure", OccurrenceCount: 3},
		Occurrences: []domain.IncidentOccurrence{
			{Ref: "main", SHA: "aaa", JobName: "test", Stage: "test", GitLabJobID: 9, GitLabPipelineID: 55},
		},
		AffectedRefs: []domain.RefSummary{{Ref: "main", OccurrenceCount: 3, DistinctSHAs: 2}},
		Signals:      domain.IncidentSignals{DistinctRefs: 1, DistinctSHAs: 2, FlakySuspected: true, RegressionSuspected: true},
	}}
	router := NewRouter(Deps{Readiness: fakeChecker{}, Incidents: lister, APIKeys: stubAuthenticator{}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/1001/incidents/abc?limit=25", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body incidentDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Fingerprint != "abc" || len(body.Occurrences) != 1 {
		t.Errorf("body = %+v, want fingerprint abc with 1 occurrence", body)
	}
	if body.Occurrences[0].Ref != "main" || body.Occurrences[0].GitLabPipelineID != 55 {
		t.Errorf("occurrence = %+v, want main / pipeline 55", body.Occurrences[0])
	}
	if !body.Signals.FlakySuspected || !body.Signals.RegressionSuspected {
		t.Errorf("signals = %+v, want flaky and regression suspected", body.Signals)
	}
	if lister.gotDetailGitlabID != 1001 || lister.gotFingerprint != "abc" || lister.gotDetailLimit != 25 {
		t.Errorf("passed (gitlabID=%d, fp=%q, limit=%d), want (1001, abc, 25)",
			lister.gotDetailGitlabID, lister.gotFingerprint, lister.gotDetailLimit)
	}
}

func TestGetIncident_UnknownProject(t *testing.T) {
	lister := &fakeIncidentLister{detailErr: application.ErrProjectNotFound}
	router := NewRouter(Deps{Readiness: fakeChecker{}, Incidents: lister, APIKeys: stubAuthenticator{}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/9999/incidents/abc", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetIncident_UnknownFingerprint(t *testing.T) {
	lister := &fakeIncidentLister{detailErr: application.ErrIncidentNotFound}
	router := NewRouter(Deps{Readiness: fakeChecker{}, Incidents: lister, APIKeys: stubAuthenticator{}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/1001/incidents/nope", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetIncident_InvalidID(t *testing.T) {
	router := NewRouter(Deps{Readiness: fakeChecker{}, Incidents: &fakeIncidentLister{}, APIKeys: stubAuthenticator{}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/bad/incidents/abc", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
