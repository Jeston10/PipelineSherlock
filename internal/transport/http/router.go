package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

// contextKey is an unexported type for request-scoped context keys so they can
// never collide with keys from other packages.
type contextKey string

const requestIDKey contextKey = "request_id"

// NewRouter builds the HTTP handler for the service: the route table plus the
// middleware chain (panic recovery, request id, structured access logging).
//
// It takes small consumer-side interfaces (ReadinessChecker, ProjectService,
// WebhookIngestor) via Deps rather than concrete types, keeping the transport
// layer decoupled from the adapters and use cases.
func NewRouter(deps Deps) http.Handler {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	api := &apiHandlers{deps: deps, logger: logger}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", LivenessHandler())
	mux.HandleFunc("GET /readyz", ReadinessHandler(deps.Readiness))
	mux.HandleFunc("POST /api/v1/projects", api.registerProject)
	mux.HandleFunc("GET /api/v1/projects", api.listProjects)
	mux.HandleFunc("GET /api/v1/projects/{gitlab_project_id}/incidents", api.listIncidents)
	mux.HandleFunc("GET /api/v1/projects/{gitlab_project_id}/incidents/{fingerprint}", api.getIncident)
	mux.HandleFunc("POST /api/v1/webhooks/gitlab", api.gitlabWebhook)

	// Middleware is applied outermost-first: recovery wraps logging wraps the
	// request-id assignment wraps the mux.
	return recoverMiddleware(logger)(
		logMiddleware(logger)(
			requestIDMiddleware(mux),
		),
	)
}

// requestIDMiddleware assigns a request id (honouring an inbound
// X-Request-ID), stores it in the context and echoes it in the response.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// logMiddleware records one structured line per request. It never logs request
// bodies, headers or query strings, which can carry secrets.
func logMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			logger.LogAttrs(r.Context(), slog.LevelInfo, "http_request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", RequestIDFromContext(r.Context())),
			)
		})
	}
}

// recoverMiddleware turns a panic in a handler into a 500 response and a logged
// error instead of crashing the server.
func recoverMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.LogAttrs(r.Context(), slog.LevelError, "http_panic",
						slog.Any("panic", rec),
						slog.String("request_id", RequestIDFromContext(r.Context())),
					)
					writeJSON(w, http.StatusInternalServerError, healthResponse{Status: "error"})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// statusRecorder captures the response status code for access logging.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fall back to a time-based id; uniqueness is best-effort for tracing.
		return time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(b[:])
}
