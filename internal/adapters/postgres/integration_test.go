//go:build integration

// Package postgres integration tests exercise the repositories against a real
// PostgreSQL instance. They are gated behind the "integration" build tag and
// skip unless DATABASE_URL is set, so the default `go test ./...` never needs a
// database. CI runs them with `go test -tags=integration` against its Postgres
// service.
package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
	pgplatform "github.com/Jeston10/pipeline-sherlock/internal/platform/postgres"
)

const pipelinePayload = `{"object_kind":"pipeline","project":{"id":1001},` +
	`"object_attributes":{"id":55,"ref":"main","sha":"deadbeef","status":"failed"},` +
	`"builds":[{"id":9,"name":"test","stage":"test","status":"failed"}]}`

func TestIntegration_InboxAndPipelineLifecycle(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping live-DB integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := pgplatform.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	if err := pgplatform.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Deterministic starting point.
	if _, err := db.Pool.Exec(ctx, "TRUNCATE projects, webhook_events, pipelines, jobs CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	projects := NewProjectRepo(db.Pool)
	events := NewWebhookEventRepo(db.Pool)
	pipelines := NewPipelineRepo(db.Pool)

	// Register a project.
	proj, err := domain.NewProject(1001, "checkout-service", "team/platform")
	if err != nil {
		t.Fatalf("new project: %v", err)
	}
	proj = proj.WithWebhookSecretHash("a-hash")
	stored, err := projects.Create(ctx, proj)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	// Insert a webhook event, then redeliver it (must de-duplicate).
	ev, err := domain.NewWebhookEvent(stored.ID, "Pipeline Hook", "delivery-1", []byte(pipelinePayload))
	if err != nil {
		t.Fatalf("new event: %v", err)
	}
	inserted, err := events.Insert(ctx, ev)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if !inserted {
		t.Fatal("first insert should write a row")
	}
	again, err := events.Insert(ctx, ev)
	if err != nil {
		t.Fatalf("redeliver insert: %v", err)
	}
	if again {
		t.Error("redelivered event should be de-duplicated")
	}

	// Claim it: status -> processing, attempts -> 1.
	claimed, ok, err := events.ClaimNext(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !ok {
		t.Fatal("expected to claim the queued event")
	}
	if claimed.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", claimed.Attempts)
	}

	// Upsert the pipeline + jobs, twice, to prove idempotency.
	pl, err := domain.NewPipeline(stored.ID, 55, "main", "deadbeef", "failed")
	if err != nil {
		t.Fatalf("new pipeline: %v", err)
	}
	jobs := []domain.Job{{GitLabJobID: 9, Name: "test", Stage: "test", Status: "failed"}}
	for i := 0; i < 2; i++ {
		if err := pipelines.Upsert(ctx, pl, jobs); err != nil {
			t.Fatalf("upsert #%d: %v", i+1, err)
		}
	}
	var pipelineCount, jobCount int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM pipelines").Scan(&pipelineCount); err != nil {
		t.Fatalf("count pipelines: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM jobs").Scan(&jobCount); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if pipelineCount != 1 || jobCount != 1 {
		t.Errorf("rows = (%d pipelines, %d jobs), want exactly 1 each after idempotent upsert", pipelineCount, jobCount)
	}

	// Mark processed; the inbox should then be empty.
	if err := events.MarkProcessed(ctx, claimed.ID); err != nil {
		t.Fatalf("mark processed: %v", err)
	}
	if _, ok, err := events.ClaimNext(ctx); err != nil || ok {
		t.Errorf("ClaimNext after processed = (ok %v, err %v), want (false, nil)", ok, err)
	}
}
