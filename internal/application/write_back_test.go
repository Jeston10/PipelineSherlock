package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

type stubCommentRepo struct {
	enqueueID      string
	enqueueCreated bool
	enqueueErr     error
	enqueued       []domain.Comment

	markPostedID     string
	markPostedNoteID int64
	markPostedErr    error
	markPostedCalls  int

	markFailedID    string
	markFailedErr   error
	markFailedCalls int
}

func (s *stubCommentRepo) Enqueue(_ context.Context, c domain.Comment) (string, bool, error) {
	s.enqueued = append(s.enqueued, c)
	return s.enqueueID, s.enqueueCreated, s.enqueueErr
}

func (s *stubCommentRepo) MarkPosted(_ context.Context, id string, gitlabNoteID int64) error {
	s.markPostedCalls++
	s.markPostedID = id
	s.markPostedNoteID = gitlabNoteID
	return s.markPostedErr
}

func (s *stubCommentRepo) MarkFailed(_ context.Context, id string) error {
	s.markFailedCalls++
	s.markFailedID = id
	return s.markFailedErr
}

type stubCommenter struct {
	noteID        int64
	err           error
	calls         int
	lastBody      string
	lastProjectID int64
	lastMRIID     int64
}

func (s *stubCommenter) PostMergeRequestNote(_ context.Context, gitlabProjectID, mergeRequestIID int64, body string) (int64, error) {
	s.calls++
	s.lastProjectID = gitlabProjectID
	s.lastMRIID = mergeRequestIID
	s.lastBody = body
	return s.noteID, s.err
}

func validWriteBackInput() WriteBackInput {
	return WriteBackInput{
		FingerprintID:   "fp-1",
		GitLabProjectID: 1001,
		MergeRequestIID: 42,
		Category:        "test_failure",
		SignatureLine:   "--- FAIL: TestLogin",
		OccurrenceCount: 4,
	}
}

func TestWriteBackPostsAndMarksPosted(t *testing.T) {
	repo := &stubCommentRepo{enqueueID: "c1", enqueueCreated: true}
	commenter := &stubCommenter{noteID: 987}
	svc := NewWriteBackService(repo, commenter, nil)

	posted, err := svc.PostDiagnosis(context.Background(), validWriteBackInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !posted {
		t.Fatalf("posted = false, want true")
	}
	if commenter.calls != 1 {
		t.Errorf("commenter calls = %d, want 1", commenter.calls)
	}
	if commenter.lastProjectID != 1001 || commenter.lastMRIID != 42 {
		t.Errorf("posted to project/mr %d/%d, want 1001/42", commenter.lastProjectID, commenter.lastMRIID)
	}
	if !strings.Contains(commenter.lastBody, "test_failure") {
		t.Errorf("body missing category: %q", commenter.lastBody)
	}
	if repo.markPostedCalls != 1 {
		t.Errorf("MarkPosted calls = %d, want 1", repo.markPostedCalls)
	}
	if repo.markPostedID != "c1" || repo.markPostedNoteID != 987 {
		t.Errorf("MarkPosted(%q, %d), want (c1, 987)", repo.markPostedID, repo.markPostedNoteID)
	}
	if repo.markFailedCalls != 0 {
		t.Errorf("MarkFailed calls = %d, want 0", repo.markFailedCalls)
	}
}

func TestWriteBackIsIdempotentWhenAlreadyEnqueued(t *testing.T) {
	repo := &stubCommentRepo{enqueueID: "c1", enqueueCreated: false}
	commenter := &stubCommenter{noteID: 987}
	svc := NewWriteBackService(repo, commenter, nil)

	posted, err := svc.PostDiagnosis(context.Background(), validWriteBackInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if posted {
		t.Errorf("posted = true, want false (idempotent skip)")
	}
	if commenter.calls != 0 {
		t.Errorf("commenter calls = %d, want 0 (must not post again)", commenter.calls)
	}
	if repo.markPostedCalls != 0 || repo.markFailedCalls != 0 {
		t.Errorf("status updated on idempotent skip: posted=%d failed=%d", repo.markPostedCalls, repo.markFailedCalls)
	}
}

func TestWriteBackMarksFailedOnPostError(t *testing.T) {
	repo := &stubCommentRepo{enqueueID: "c1", enqueueCreated: true}
	commenter := &stubCommenter{err: errors.New("gitlab 500")}
	svc := NewWriteBackService(repo, commenter, nil)

	posted, err := svc.PostDiagnosis(context.Background(), validWriteBackInput())
	if err == nil {
		t.Fatalf("expected an error when the post fails")
	}
	if posted {
		t.Errorf("posted = true, want false")
	}
	if repo.markFailedCalls != 1 || repo.markFailedID != "c1" {
		t.Errorf("MarkFailed calls=%d id=%q, want 1/c1", repo.markFailedCalls, repo.markFailedID)
	}
	if repo.markPostedCalls != 0 {
		t.Errorf("MarkPosted calls = %d, want 0", repo.markPostedCalls)
	}
}

func TestWriteBackRejectsInvalidInputWithoutEnqueue(t *testing.T) {
	repo := &stubCommentRepo{enqueueID: "c1", enqueueCreated: true}
	commenter := &stubCommenter{}
	svc := NewWriteBackService(repo, commenter, nil)

	in := validWriteBackInput()
	in.FingerprintID = ""

	posted, err := svc.PostDiagnosis(context.Background(), in)
	if !errors.Is(err, domain.ErrInvalidComment) {
		t.Fatalf("error = %v, want ErrInvalidComment", err)
	}
	if posted {
		t.Errorf("posted = true, want false")
	}
	if len(repo.enqueued) != 0 {
		t.Errorf("Enqueue called %d times, want 0 for invalid input", len(repo.enqueued))
	}
	if commenter.calls != 0 {
		t.Errorf("commenter calls = %d, want 0", commenter.calls)
	}
}

func TestRenderIncidentComment(t *testing.T) {
	body := RenderIncidentComment(WriteBackInput{
		Category:        "test_failure",
		SignatureLine:   "--- FAIL: TestLogin",
		OccurrenceCount: 4,
		IncidentURL:     "https://sherlock.example/incidents/fp-1",
	})

	for _, want := range []string{"test_failure", "--- FAIL: TestLogin", "4", "https://sherlock.example/incidents/fp-1"} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered body missing %q:\n%s", want, body)
		}
	}
}

func TestRenderIncidentCommentOmitsEmptyURL(t *testing.T) {
	body := RenderIncidentComment(WriteBackInput{Category: "test_failure", OccurrenceCount: 1})
	if strings.Contains(body, "View incident history") {
		t.Errorf("body should omit the link when IncidentURL is empty:\n%s", body)
	}
}
