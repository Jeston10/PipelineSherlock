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
}

func (f *fakeIncidentLister) List(_ context.Context, gitlabProjectID int64, limit int) ([]domain.Incident, error) {
	f.gotGitlabID = gitlabProjectID
	f.gotLimit = limit
	return f.incidents, f.err
}

func TestListIncidentsEndpoint(t *testing.T) {
	lister := &fakeIncidentLister{incidents: []domain.Incident{{
		Fingerprint:     "fp1",
		Category:        "test_failure",
		OccurrenceCount: 4,
		FirstSeenAt:     time.Unix(1000, 0).UTC(),
		LastSeenAt:      time.Unix(2000, 0).UTC(),
	}}}
	router := NewRouter(Deps{Readiness: fakeChecker{}, Incidents: lister})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/1001/incidents?limit=10", nil)
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
	router := NewRouter(Deps{Readiness: fakeChecker{}, Incidents: lister})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/9999/incidents", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestListIncidents_InvalidID(t *testing.T) {
	router := NewRouter(Deps{Readiness: fakeChecker{}, Incidents: &fakeIncidentLister{}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/not-a-number/incidents", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
