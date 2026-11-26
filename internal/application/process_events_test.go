package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// fakePipelineRepo records upserts and can be made to fail. It is mutex-guarded
// because the worker calls Upsert from a separate goroutine.
type fakePipelineRepo struct {
	mu      sync.Mutex
	upserts []upsertCall
	err     error
}

type upsertCall struct {
	pipeline domain.Pipeline
	jobs     []domain.Job
}

func (r *fakePipelineRepo) Upsert(_ context.Context, p domain.Pipeline, jobs []domain.Job) (map[int64]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	r.upserts = append(r.upserts, upsertCall{pipeline: p, jobs: jobs})
	ids := make(map[int64]string, len(jobs))
	for _, j := range jobs {
		ids[j.GitLabJobID] = fmt.Sprintf("job-%d", j.GitLabJobID)
	}
	return ids, nil
}

// count returns the number of recorded upserts under the lock.
func (r *fakePipelineRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.upserts)
}

// fakeGitLabClient returns programmed authoritative state.
type fakeGitLabClient struct {
	pipeline GitLabPipeline
	jobs     []GitLabJob
	err      error
	trace    string
	traceErr error
}

func (c fakeGitLabClient) GetPipeline(context.Context, int64, int64) (GitLabPipeline, error) {
	return c.pipeline, c.err
}
func (c fakeGitLabClient) ListPipelineJobs(context.Context, int64, int64) ([]GitLabJob, error) {
	return c.jobs, c.err
}
func (c fakeGitLabClient) GetJobTrace(context.Context, int64, int64) (string, error) {
	return c.trace, c.traceErr
}

// fakeFingerprintRepo records occurrences and can be made to fail.
type fakeFingerprintRepo struct {
	mu          sync.Mutex
	occurrences []FingerprintOccurrence
	err         error
}

func (r *fakeFingerprintRepo) RecordOccurrence(_ context.Context, occ FingerprintOccurrence) (RecordOccurrenceResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return RecordOccurrenceResult{}, r.err
	}
	r.occurrences = append(r.occurrences, occ)
	return RecordOccurrenceResult{
		FingerprintID:   "fp-" + occ.Fingerprint,
		OccurrenceCount: int64(len(r.occurrences)),
		NewOccurrence:   true,
	}, nil
}

func (r *fakeFingerprintRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.occurrences)
}

const pipelinePayloadJSON = `{
	"object_kind":"pipeline",
	"project":{"id":1001},
	"object_attributes":{"id":55,"ref":"main","sha":"deadbeef","status":"failed"},
	"builds":[{"id":9,"name":"test","stage":"test","status":"failed"}]
}`

func mustInsert(t *testing.T, repo *fakeEventRepo, eventType, payload string) {
	t.Helper()
	ev, err := domain.NewWebhookEvent("proj_1", eventType, "d-"+eventType, []byte(payload))
	if err != nil {
		t.Fatalf("new event: %v", err)
	}
	if _, err := repo.Insert(context.Background(), ev); err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestProcessNext_PayloadFallback(t *testing.T) {
	ctx := context.Background()
	events := newFakeEventRepo()
	pipelines := &fakePipelineRepo{}
	mustInsert(t, events, "Pipeline Hook", pipelinePayloadJSON)

	svc := NewProcessEventsService(events, pipelines, nil, nil, nil)

	progressed, err := svc.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !progressed {
		t.Fatal("expected progress on a non-empty inbox")
	}
	if len(pipelines.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(pipelines.upserts))
	}
	got := pipelines.upserts[0]
	if got.pipeline.GitLabPipelineID != 55 || got.pipeline.Status != "failed" {
		t.Errorf("pipeline = %+v, want id 55 status failed", got.pipeline)
	}
	if len(got.jobs) != 1 || got.jobs[0].Name != "test" {
		t.Errorf("jobs = %+v, want one job 'test'", got.jobs)
	}
	if events.events[0].Status != domain.WebhookStatusProcessed {
		t.Errorf("event status = %q, want processed", events.events[0].Status)
	}
}

func TestProcessNext_ReconcilesViaGitLab(t *testing.T) {
	ctx := context.Background()
	events := newFakeEventRepo()
	pipelines := &fakePipelineRepo{}
	mustInsert(t, events, "Pipeline Hook", pipelinePayloadJSON)

	// GitLab reports the pipeline has since succeeded; this must win over the
	// "failed" status in the (stale) payload.
	gitlab := fakeGitLabClient{
		pipeline: GitLabPipeline{ID: 55, Ref: "main", SHA: "deadbeef", Status: "success"},
		jobs:     []GitLabJob{{ID: 9, Name: "test", Stage: "test", Status: "success"}},
	}
	svc := NewProcessEventsService(events, pipelines, nil, gitlab, nil)

	if _, err := svc.ProcessNext(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pipelines.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(pipelines.upserts))
	}
	if got := pipelines.upserts[0].pipeline.Status; got != "success" {
		t.Errorf("status = %q, want success (authoritative GitLab state)", got)
	}
}

func TestProcessNext_FingerprintsFailedJob(t *testing.T) {
	ctx := context.Background()
	events := newFakeEventRepo()
	pipelines := &fakePipelineRepo{}
	fingerprints := &fakeFingerprintRepo{}
	mustInsert(t, events, "Pipeline Hook", pipelinePayloadJSON)

	gitlab := fakeGitLabClient{
		pipeline: GitLabPipeline{ID: 55, Ref: "main", SHA: "deadbeef", Status: "failed"},
		jobs: []GitLabJob{
			{ID: 9, Name: "test", Stage: "test", Status: "failed"},
			{ID: 10, Name: "build", Stage: "build", Status: "success"},
		},
		trace: "--- FAIL: TestLogin (0.01s)\n    login_test.go:21: status = 500, want 200",
	}
	svc := NewProcessEventsService(events, pipelines, fingerprints, gitlab, nil)

	if _, err := svc.ProcessNext(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fingerprints.count() != 1 {
		t.Fatalf("occurrences = %d, want 1 (only the failed job)", fingerprints.count())
	}
	occ := fingerprints.occurrences[0]
	if occ.JobID != "job-9" {
		t.Errorf("JobID = %q, want job-9 (the failed job's row id)", occ.JobID)
	}
	if occ.Category != "test_failure" {
		t.Errorf("category = %q, want test_failure", occ.Category)
	}
	if occ.Fingerprint == "" || occ.EvidenceHash == "" {
		t.Errorf("fingerprint/evidence hash should be set: %+v", occ)
	}
}

// A trace-fetch failure must not fail the event: pipeline state is already
// persisted, so fingerprinting is best-effort.
func TestProcessNext_FingerprintBestEffortOnTraceError(t *testing.T) {
	ctx := context.Background()
	events := newFakeEventRepo()
	pipelines := &fakePipelineRepo{}
	fingerprints := &fakeFingerprintRepo{}
	mustInsert(t, events, "Pipeline Hook", pipelinePayloadJSON)

	gitlab := fakeGitLabClient{
		pipeline: GitLabPipeline{ID: 55, Ref: "main", SHA: "deadbeef", Status: "failed"},
		jobs:     []GitLabJob{{ID: 9, Name: "test", Stage: "test", Status: "failed"}},
		traceErr: errors.New("gitlab 500"),
	}
	svc := NewProcessEventsService(events, pipelines, fingerprints, gitlab, nil)

	if _, err := svc.ProcessNext(ctx); err != nil {
		t.Fatalf("event should still succeed despite trace error, got: %v", err)
	}
	if fingerprints.count() != 0 {
		t.Errorf("occurrences = %d, want 0 (trace fetch failed)", fingerprints.count())
	}
	if events.events[0].Status != domain.WebhookStatusProcessed {
		t.Errorf("event status = %q, want processed", events.events[0].Status)
	}
}

func TestProcessNext_WriteBackOnMergeRequestPipeline(t *testing.T) {
	ctx := context.Background()
	events := newFakeEventRepo()
	pipelines := &fakePipelineRepo{}
	fingerprints := &fakeFingerprintRepo{}

	const mrPayload = `{
		"object_kind":"pipeline",
		"project":{"id":1001},
		"object_attributes":{"id":55,"ref":"feature","sha":"abc","status":"failed"},
		"merge_request":{"iid":42},
		"builds":[{"id":9,"name":"test","stage":"test","status":"failed"}]
	}`
	mustInsert(t, events, "Pipeline Hook", mrPayload)

	gitlab := fakeGitLabClient{
		pipeline: GitLabPipeline{ID: 55, Ref: "feature", SHA: "abc", Status: "failed"},
		jobs:     []GitLabJob{{ID: 9, Name: "test", Stage: "test", Status: "failed"}},
		trace:    "--- FAIL: TestLogin (0.01s)\n    login_test.go:21: status = 500, want 200",
	}
	commenter := &stubCommenter{noteID: 7}
	wb := NewWriteBackService(&stubCommentRepo{enqueueID: "c1", enqueueCreated: true}, commenter, nil)
	svc := NewProcessEventsService(events, pipelines, fingerprints, gitlab, nil, WithWriteBack(wb))

	if _, err := svc.ProcessNext(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commenter.calls != 1 {
		t.Fatalf("write-back posts = %d, want 1", commenter.calls)
	}
	if commenter.lastProjectID != 1001 || commenter.lastMRIID != 42 {
		t.Errorf("posted to project/mr %d/%d, want 1001/42", commenter.lastProjectID, commenter.lastMRIID)
	}
}

func TestProcessNext_NoWriteBackWithoutMergeRequest(t *testing.T) {
	ctx := context.Background()
	events := newFakeEventRepo()
	pipelines := &fakePipelineRepo{}
	fingerprints := &fakeFingerprintRepo{}
	mustInsert(t, events, "Pipeline Hook", pipelinePayloadJSON) // payload carries no merge_request

	gitlab := fakeGitLabClient{
		pipeline: GitLabPipeline{ID: 55, Ref: "main", SHA: "deadbeef", Status: "failed"},
		jobs:     []GitLabJob{{ID: 9, Name: "test", Stage: "test", Status: "failed"}},
		trace:    "--- FAIL: TestLogin",
	}
	commenter := &stubCommenter{noteID: 7}
	wb := NewWriteBackService(&stubCommentRepo{enqueueID: "c1", enqueueCreated: true}, commenter, nil)
	svc := NewProcessEventsService(events, pipelines, fingerprints, gitlab, nil, WithWriteBack(wb))

	if _, err := svc.ProcessNext(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commenter.calls != 0 {
		t.Errorf("write-back posts = %d, want 0 (no merge request)", commenter.calls)
	}
}

func TestProcessNext_NonPipelineEventSkipped(t *testing.T) {
	ctx := context.Background()
	events := newFakeEventRepo()
	pipelines := &fakePipelineRepo{}
	mustInsert(t, events, "Push Hook", `{"object_kind":"push","project":{"id":1001}}`)

	svc := NewProcessEventsService(events, pipelines, nil, nil, nil)

	progressed, err := svc.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !progressed {
		t.Fatal("expected progress")
	}
	if len(pipelines.upserts) != 0 {
		t.Errorf("upserts = %d, want 0 for a non-pipeline event", len(pipelines.upserts))
	}
	if events.events[0].Status != domain.WebhookStatusProcessed {
		t.Errorf("event status = %q, want processed", events.events[0].Status)
	}
}

func TestProcessNext_EmptyInbox(t *testing.T) {
	events := newFakeEventRepo()
	svc := NewProcessEventsService(events, &fakePipelineRepo{}, nil, nil, nil)

	progressed, err := svc.ProcessNext(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if progressed {
		t.Error("expected no progress on an empty inbox")
	}
}

func TestProcessNext_RequeuesThenFails(t *testing.T) {
	ctx := context.Background()
	events := newFakeEventRepo()
	pipelines := &fakePipelineRepo{err: errors.New("db down")}
	mustInsert(t, events, "Pipeline Hook", pipelinePayloadJSON)

	svc := NewProcessEventsService(events, pipelines, nil, nil, nil)
	svc.maxAttempts = 2 // white-box: keep the test short

	// Attempt 1 fails → requeued (back to received).
	if _, err := svc.ProcessNext(ctx); err != nil {
		t.Fatalf("attempt 1: %v", err)
	}
	if events.events[0].Status != domain.WebhookStatusReceived {
		t.Fatalf("after attempt 1 status = %q, want received (requeued)", events.events[0].Status)
	}

	// Attempt 2 reaches the bound → failed terminally.
	if _, err := svc.ProcessNext(ctx); err != nil {
		t.Fatalf("attempt 2: %v", err)
	}
	if events.events[0].Status != domain.WebhookStatusFailed {
		t.Errorf("after attempt 2 status = %q, want failed", events.events[0].Status)
	}
}
