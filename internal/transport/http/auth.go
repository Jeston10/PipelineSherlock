package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Jeston10/pipeline-sherlock/internal/application"
	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// APIKeyAuthenticator verifies a presented API key and returns the matching key
// record, or application.ErrUnauthenticated when the key is absent or unknown.
// The application APIKeyService satisfies it; tests provide fakes. Defined here,
// consumer-side, so the transport layer is not bound to a concrete service.
type APIKeyAuthenticator interface {
	Authenticate(ctx context.Context, plaintextKey string) (domain.APIKey, error)
}

const bearerPrefix = "Bearer "

// authMiddleware enforces API-key authentication on protected /api/v1 routes.
// Public routes pass through untouched: the health probes (not under /api/v1)
// and the GitLab webhook, which authenticates GitLab with its own per-project
// token. A protected route with no configured authenticator fails closed (401).
func authMiddleware(authenticator APIKeyAuthenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !requiresAuth(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			key, ok := bearerToken(r)
			if !ok || authenticator == nil {
				writeUnauthorized(w)
				return
			}

			authed, err := authenticator.Authenticate(r.Context(), key)
			if err != nil {
				if errors.Is(err, application.ErrUnauthenticated) {
					writeUnauthorized(w)
					return
				}
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
				return
			}
			next.ServeHTTP(w, r.WithContext(contextWithOwnerID(r.Context(), authed.OwnerID)))
		})
	}
}

// requiresAuth reports whether a path is a protected API route. The webhook
// endpoint carries its own per-project token, so it is public here; the health
// probes are not under /api/v1 and are always public.
func requiresAuth(path string) bool {
	return strings.HasPrefix(path, "/api/v1/") && !strings.HasPrefix(path, "/api/v1/webhooks/")
}

// bearerToken extracts a non-empty bearer token from the Authorization header.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, bearerPrefix) {
		return "", false
	}
	key := strings.TrimSpace(strings.TrimPrefix(h, bearerPrefix))
	if key == "" {
		return "", false
	}
	return key, true
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
}
