package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeChecker is a test double for ReadinessChecker.
type fakeChecker struct{ err error }

func (f fakeChecker) Ping(context.Context) error { return f.err }

func TestLivenessHandler_AlwaysOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	LivenessHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status field = %q, want ok", body.Status)
	}
}

func TestReadinessHandler(t *testing.T) {
	tests := []struct {
		name       string
		checker    ReadinessChecker
		wantStatus int
		wantOK     string
		wantDBMark string
	}{
		{
			name:       "database reachable",
			checker:    fakeChecker{err: nil},
			wantStatus: http.StatusOK,
			wantOK:     "ok",
			wantDBMark: "ok",
		},
		{
			name:       "database unavailable",
			checker:    fakeChecker{err: errors.New("connection refused")},
			wantStatus: http.StatusServiceUnavailable,
			wantOK:     "unavailable",
			wantDBMark: "unavailable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			rec := httptest.NewRecorder()

			ReadinessHandler(tc.checker).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			var body healthResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.Status != tc.wantOK {
				t.Errorf("status field = %q, want %q", body.Status, tc.wantOK)
			}
			if got := body.Checks["database"]; got != tc.wantDBMark {
				t.Errorf("checks.database = %q, want %q", got, tc.wantDBMark)
			}
		})
	}
}

// TestReadinessHandler_NeverLeaksErrorDetail ensures the readiness body does
// not echo raw dependency error strings, which can carry host/credential
// detail.
func TestReadinessHandler_NeverLeaksErrorDetail(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	ReadinessHandler(fakeChecker{err: errors.New("dial tcp 10.0.0.5:5432: secret-in-error")}).ServeHTTP(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, "secret-in-error") || strings.Contains(body, "10.0.0.5") {
		t.Errorf("readiness body leaked dependency error detail: %s", body)
	}
}
