package application

import (
	"context"
	"errors"
	"testing"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// fakePipelineRepo records upserts and can be made to fail.
type fakePipelineRepo struct {
	upserts []upsertCall
	err     error
}

type upsertCall struct {
	pipeline domain.Pipeline
	jobs     []domain.Job
}

func (r *fakePipelineRepo) Upsert(_ context.Context, p domain.Pipeline, jobs []domain.Job) error {
	if r.err != nil {
		return r.err
	}
	r.upserts = append(r.upserts, upsertCall{pipeline: p, jobs: jobs})
	return nil
}

// fakeGitLabClient returns programmed authoritative state.
type fakeGitLabClient struct {
	pipeline GitLabPipeline
	jobs     []GitLabJob
	err      error
}

func (c fakeGitLabClient) GetPipeline(context.Context, int64, int64) (GitLabPipeline, error) {
	return c.pipeline, c.err
}
func (c fakeGitLabClient) ListPipelineJobs(context.Context, int64, int64) ([]GitLabJob, error) {
	return c.jobs, c.err
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

	svc := NewProcessEventsService(events, pipelines, nil, nil)

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
	svc := NewProcessEventsService(events, pipelines, gitlab, nil)

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

func TestProcessNext_NonPipelineEventSkipped(t *testing.T) {
	ctx := context.Background()
	events := newFakeEventRepo()
	pipelines := &fakePipelineRepo{}
	mustInsert(t, events, "Push Hook", `{"object_kind":"push","project":{"id":1001}}`)

	svc := NewProcessEventsService(events, pipelines, nil, nil)

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
	svc := NewProcessEventsService(events, &fakePipelineRepo{}, nil, nil)

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

	svc := NewProcessEventsService(events, pipelines, nil, nil)
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
