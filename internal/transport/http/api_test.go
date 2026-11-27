package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jeston10/pipeline-sherlock/internal/application"
	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// fakeProjectService implements ProjectService.
type fakeProjectService struct {
	result application.RegisterProjectResult
	err    error
	list   []domain.Project
}

func (f fakeProjectService) Register(context.Context, int64, string, string) (application.RegisterProjectResult, error) {
	return f.result, f.err
}
func (f fakeProjectService) List(context.Context) ([]domain.Project, error) {
	return f.list, nil
}

// fakeIngestor implements WebhookIngestor with a programmable response.
type fakeIngestor struct {
	result application.IngestResult
	err    error
}

func (f fakeIngestor) Ingest(context.Context, application.IngestCommand) (application.IngestResult, error) {
	return f.result, f.err
}

func newTestRouter(projects ProjectService, webhooks WebhookIngestor) http.Handler {
	return NewRouter(Deps{
		Readiness: fakeChecker{},
		Projects:  projects,
		Webhooks:  webhooks,
		APIKeys:   stubAuthenticator{},
	})
}

func TestRegisterProjectEndpoint(t *testing.T) {
	p, _ := domain.NewProject(1001, "svc", "team")
	p = p.WithPersistence("proj_1", p.CreatedAt)
	router := newTestRouter(
		fakeProjectService{result: application.RegisterProjectResult{Project: p, WebhookSecret: "plain-secret"}},
		fakeIngestor{},
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects",
		strings.NewReader(`{"gitlab_project_id":1001,"name":"svc","namespace":"team"}`))
	req.Header.Set("Authorization", "Bearer test-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var body projectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.WebhookSecret != "plain-secret" {
		t.Errorf("webhook_secret = %q, want the one-time secret", body.WebhookSecret)
	}
}

func TestGitLabWebhookEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		ingestor   fakeIngestor
		wantStatus int
	}{
		{"accepted", fakeIngestor{result: application.IngestResult{Accepted: true}}, http.StatusAccepted},
		{"duplicate", fakeIngestor{result: application.IngestResult{Accepted: false}}, http.StatusAccepted},
		{"invalid token", fakeIngestor{err: application.ErrInvalidSignature}, http.StatusUnauthorized},
		{"unknown project", fakeIngestor{err: application.ErrProjectNotFound}, http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := newTestRouter(fakeProjectService{}, tc.ingestor)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/gitlab",
				strings.NewReader(`{"object_kind":"pipeline","project":{"id":1001}}`))
			req.Header.Set("X-Gitlab-Event", "Pipeline Hook")
			req.Header.Set("X-Gitlab-Token", "some-token")
			req.Header.Set("X-Gitlab-Event-UUID", "delivery-xyz")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestGitLabWebhook_MissingEventHeader(t *testing.T) {
	router := newTestRouter(fakeProjectService{}, fakeIngestor{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/gitlab",
		strings.NewReader(`{"project":{"id":1001}}`))
	// No X-Gitlab-Event header.
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
