// Package http is the HTTP transport layer: routing, middleware and handlers.
// It depends on the application/domain layers through small interfaces and
// never contains business rules itself.
package http

import (
	"context"
	"encoding/json"
	"net/http"
)

// ReadinessChecker reports whether a downstream dependency is usable. The
// Postgres adapter satisfies it; tests provide fakes. The interface is defined
// here, where it is consumed (Go idiom), not where it is implemented.
type ReadinessChecker interface {
	Ping(ctx context.Context) error
}

// healthResponse is the body returned by the liveness and readiness endpoints.
type healthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// LivenessHandler reports that the process is up and serving. It performs no
// dependency checks by design: a failing liveness probe should mean "restart
// me", not "a dependency is down".
func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
	}
}

// ReadinessHandler reports whether the service can serve real traffic, which
// requires its dependencies (the database) to be reachable. It returns 503 when
// a dependency check fails so load balancers stop routing to this instance.
func ReadinessHandler(db ReadinessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]string{}
		status := http.StatusOK
		overall := "ok"

		if err := db.Ping(r.Context()); err != nil {
			checks["database"] = "unavailable"
			status = http.StatusServiceUnavailable
			overall = "unavailable"
		} else {
			checks["database"] = "ok"
		}

		writeJSON(w, status, healthResponse{Status: overall, Checks: checks})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// Encoding a fixed, non-user struct cannot realistically fail; if it did
	// the status line is already written, so there is nothing further to do.
	_ = json.NewEncoder(w).Encode(body)
}
