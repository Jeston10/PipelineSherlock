package gitlab

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testClient(server *httptest.Server) *Client {
	// Tiny base delay keeps retry tests fast.
	return New(server.URL, "test-token",
		WithHTTPClient(server.Client()),
		WithRetry(3, time.Millisecond),
	)
}

func TestGetPipeline(t *testing.T) {
	var gotToken, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("PRIVATE-TOKEN")
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"id":55,"ref":"main","sha":"deadbeef","status":"failed","duration":90,"started_at":"2026-01-01T10:00:00Z"}`)
	}))
	defer server.Close()

	p, err := testClient(server).GetPipeline(context.Background(), 1001, 55)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotToken != "test-token" {
		t.Errorf("PRIVATE-TOKEN = %q, want test-token", gotToken)
	}
	if gotPath != "/api/v4/projects/1001/pipelines/55" {
		t.Errorf("path = %q, want the pipeline endpoint", gotPath)
	}
	if p.ID != 55 || p.Status != "failed" {
		t.Errorf("pipeline = %+v, want id 55 status failed", p)
	}
	if p.Duration == nil || *p.Duration != 90 {
		t.Errorf("duration = %v, want 90", p.Duration)
	}
	if p.StartedAt == nil {
		t.Error("StartedAt should be parsed")
	}
}

func TestListPipelineJobs_Paginates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "1":
			w.Header().Set("X-Next-Page", "2")
			fmt.Fprint(w, `[{"id":1,"name":"build","stage":"build","status":"success","duration":12.4}]`)
		case "2":
			// No X-Next-Page header => last page.
			fmt.Fprint(w, `[{"id":2,"name":"test","stage":"test","status":"failed","duration":30.9}]`)
		default:
			t.Errorf("unexpected page %q", r.URL.Query().Get("page"))
		}
	}))
	defer server.Close()

	jobs, err := testClient(server).ListPipelineJobs(context.Background(), 1001, 55)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2 across pages", len(jobs))
	}
	if jobs[0].Name != "build" || jobs[1].Name != "test" {
		t.Errorf("job names = %q,%q", jobs[0].Name, jobs[1].Name)
	}
	if jobs[1].Duration == nil || *jobs[1].Duration != 31 { // 30.9 rounds to 31
		t.Errorf("duration = %v, want 31", jobs[1].Duration)
	}
}

func TestGetPipeline_RetriesOn5xx(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"id":55,"status":"success"}`)
	}))
	defer server.Close()

	p, err := testClient(server).GetPipeline(context.Background(), 1001, 55)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3 (two failures then success)", calls)
	}
	if p.Status != "success" {
		t.Errorf("status = %q, want success", p.Status)
	}
}

func TestGetPipeline_NonRetryableError(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := testClient(server).GetPipeline(context.Background(), 1001, 55)
	if err == nil {
		t.Fatal("expected an error for 404")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (404 is not retried)", calls)
	}
}
