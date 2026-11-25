package gitlab

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPostMergeRequestNote(t *testing.T) {
	var gotMethod, gotPath, gotToken, gotContentType, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotToken = r.Header.Get("PRIVATE-TOKEN")
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		var payload struct {
			Body string `json:"body"`
		}
		_ = json.Unmarshal(b, &payload)
		gotBody = payload.Body

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": 555, "body": "ignored"}`))
	}))
	defer server.Close()

	noteID, err := testClient(server).PostMergeRequestNote(context.Background(), 1001, 42, "diagnosis body")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if noteID != 555 {
		t.Errorf("noteID = %d, want 555", noteID)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/api/v4/projects/1001/merge_requests/42/notes" {
		t.Errorf("path = %s, want the MR notes path", gotPath)
	}
	if gotToken != "test-token" {
		t.Errorf("token header = %q, want test-token", gotToken)
	}
	if !strings.HasPrefix(gotContentType, "application/json") {
		t.Errorf("content-type = %q, want application/json", gotContentType)
	}
	if gotBody != "diagnosis body" {
		t.Errorf("posted body = %q, want %q", gotBody, "diagnosis body")
	}
}

func TestPostMergeRequestNote_RetriesOn5xx(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": 7}`))
	}))
	defer server.Close()

	noteID, err := testClient(server).PostMergeRequestNote(context.Background(), 1, 2, "body")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if noteID != 7 {
		t.Errorf("noteID = %d, want 7", noteID)
	}
	if calls != 2 {
		t.Errorf("server calls = %d, want 2 (one 500 then success)", calls)
	}
}

func TestPostMergeRequestNote_NonRetryableError(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := testClient(server).PostMergeRequestNote(context.Background(), 1, 2, "body")
	if err == nil {
		t.Fatalf("expected an error for a 404")
	}
	if calls != 1 {
		t.Errorf("server calls = %d, want 1 (404 is not retried)", calls)
	}
}
