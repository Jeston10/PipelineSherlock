package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jeston10/pipeline-sherlock/internal/application"
	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// stubAuthenticator is a programmable APIKeyAuthenticator for transport tests.
// The zero value authenticates any non-empty key (returns a zero key, nil err).
type stubAuthenticator struct {
	key domain.APIKey
	err error
}

func (s stubAuthenticator) Authenticate(context.Context, string) (domain.APIKey, error) {
	return s.key, s.err
}

func authRouter(auth APIKeyAuthenticator) http.Handler {
	return NewRouter(Deps{
		Readiness: fakeChecker{},
		Projects:  fakeProjectService{list: nil},
		Webhooks:  fakeIngestor{result: application.IngestResult{Accepted: true}},
		Incidents: &fakeIncidentLister{},
		APIKeys:   auth,
	})
}

func TestAuth_ProtectedRoute(t *testing.T) {
	tests := []struct {
		name       string
		auth       APIKeyAuthenticator
		authHeader string
		wantStatus int
	}{
		{"valid key", stubAuthenticator{}, "Bearer good-key", http.StatusOK},
		{"missing header", stubAuthenticator{}, "", http.StatusUnauthorized},
		{"malformed header", stubAuthenticator{}, "Token good-key", http.StatusUnauthorized},
		{"empty bearer", stubAuthenticator{}, "Bearer ", http.StatusUnauthorized},
		{"unknown key", stubAuthenticator{err: application.ErrUnauthenticated}, "Bearer nope", http.StatusUnauthorized},
		{"no authenticator configured", nil, "Bearer good-key", http.StatusUnauthorized},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := authRouter(tc.auth)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus == http.StatusUnauthorized {
				if got := rec.Header().Get("WWW-Authenticate"); got != "Bearer" {
					t.Errorf("WWW-Authenticate = %q, want Bearer", got)
				}
			}
		})
	}
}

func TestAuth_PublicRoutes(t *testing.T) {
	// Health probes and the GitLab webhook must never require an API key, even
	// with no authenticator configured.
	router := authRouter(nil)

	t.Run("healthz public", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("healthz status = %d, want 200", rec.Code)
		}
	})

	t.Run("readyz public", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("readyz status = %d, want 200", rec.Code)
		}
	})

	t.Run("gitlab webhook public", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/gitlab",
			strings.NewReader(`{"object_kind":"pipeline","project":{"id":1001}}`))
		req.Header.Set("X-Gitlab-Event", "Pipeline Hook")
		req.Header.Set("X-Gitlab-Token", "some-token")
		req.Header.Set("X-Gitlab-Event-UUID", "delivery-xyz")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code == http.StatusUnauthorized {
			t.Fatalf("webhook must be public, got 401; body=%s", rec.Body.String())
		}
	})
}
