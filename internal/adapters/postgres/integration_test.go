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

	"github.com/Jeston10/pipeline-sherlock/internal/application"
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
	var jobIDs map[int64]string
	for i := 0; i < 2; i++ {
		jobIDs, err = pipelines.Upsert(ctx, pl, jobs)
		if err != nil {
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

	// Record the same failure fingerprint twice: the occurrence and count must
	// not double (idempotent on job + fingerprint).
	fingerprints := NewFingerprintRepo(db.Pool)
	occ := application.FingerprintOccurrence{
		ProjectID:           stored.ID,
		JobID:               jobIDs[9],
		Fingerprint:         "fp-deadbeef",
		Category:            "test_failure",
		NormalizedSignature: "--- FAIL: TestLogin",
		AlgorithmVersion:    1,
		Evidence:            "--- FAIL: TestLogin\n    want 200",
		EvidenceHash:        "evhash",
	}
	for i := 0; i < 2; i++ {
		if _, err := fingerprints.RecordOccurrence(ctx, occ); err != nil {
			t.Fatalf("record occurrence #%d: %v", i+1, err)
		}
	}
	var fpCount, occCount, occurrenceCountCol int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM failure_fingerprints").Scan(&fpCount); err != nil {
		t.Fatalf("count fingerprints: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM failure_occurrences").Scan(&occCount); err != nil {
		t.Fatalf("count occurrences: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, "SELECT occurrence_count FROM failure_fingerprints LIMIT 1").Scan(&occurrenceCountCol); err != nil {
		t.Fatalf("read occurrence_count: %v", err)
	}
	if fpCount != 1 || occCount != 1 || occurrenceCountCol != 1 {
		t.Errorf("fingerprints=%d occurrences=%d occurrence_count=%d, want 1/1/1 after idempotent record",
			fpCount, occCount, occurrenceCountCol)
	}

	// The incident read model should surface that fingerprint as one incident.
	incidentRepo := NewIncidentRepo(db.Pool)
	incs, err := incidentRepo.ListByProject(ctx, stored.ID, 50)
	if err != nil {
		t.Fatalf("list incidents: %v", err)
	}
	if len(incs) != 1 {
		t.Fatalf("incidents = %d, want 1", len(incs))
	}
	if incs[0].Fingerprint != "fp-deadbeef" || incs[0].Category != "test_failure" || incs[0].OccurrenceCount != 1 {
		t.Errorf("incident = %+v, want fp-deadbeef/test_failure/count 1", incs[0])
	}

	// The incident detail read model should resolve the header by fingerprint
	// and surface the enriched occurrence history (ref/sha from the pipeline).
	header, found, err := incidentRepo.GetByFingerprint(ctx, stored.ID, "fp-deadbeef")
	if err != nil {
		t.Fatalf("get incident by fingerprint: %v", err)
	}
	if !found {
		t.Fatal("expected to find the fp-deadbeef incident")
	}
	occurrences, err := incidentRepo.ListOccurrences(ctx, header.FingerprintID, 50)
	if err != nil {
		t.Fatalf("list occurrences: %v", err)
	}
	if len(occurrences) != 1 {
		t.Fatalf("occurrences = %d, want 1", len(occurrences))
	}
	if occurrences[0].Ref != "main" || occurrences[0].SHA != "deadbeef" || occurrences[0].JobName != "test" {
		t.Errorf("occurrence = %+v, want ref main / sha deadbeef / job test", occurrences[0])
	}
	// Unknown fingerprint must be a clean not-found, not an error.
	if _, found, err := incidentRepo.GetByFingerprint(ctx, stored.ID, "does-not-exist"); err != nil || found {
		t.Errorf("GetByFingerprint(unknown) = (found %v, err %v), want (false, nil)", found, err)
	}

	// Merge-request write-back: enqueue a comment for this incident, posting it
	// at most once per (fingerprint, MR) even when enqueued twice.
	comments := NewCommentRepo(db.Pool)
	comment, err := domain.NewComment(header.FingerprintID, 42)
	if err != nil {
		t.Fatalf("new comment: %v", err)
	}
	commentID, created, err := comments.Enqueue(ctx, comment)
	if err != nil {
		t.Fatalf("enqueue comment: %v", err)
	}
	if !created || commentID == "" {
		t.Fatalf("first enqueue = (created %v, id %q), want (true, non-empty)", created, commentID)
	}
	againID, created, err := comments.Enqueue(ctx, comment)
	if err != nil {
		t.Fatalf("re-enqueue comment: %v", err)
	}
	if created || againID != commentID {
		t.Errorf("re-enqueue = (created %v, id %q), want (false, %q) — must be idempotent", created, againID, commentID)
	}
	var commentCount int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM comments").Scan(&commentCount); err != nil {
		t.Fatalf("count comments: %v", err)
	}
	if commentCount != 1 {
		t.Errorf("comments = %d, want 1 after idempotent enqueue", commentCount)
	}

	// Mark it posted and verify the persisted status, note id and timestamp.
	if err := comments.MarkPosted(ctx, commentID, 777); err != nil {
		t.Fatalf("mark posted: %v", err)
	}
	var status string
	var noteID int64
	var postedAt *time.Time
	if err := db.Pool.QueryRow(ctx,
		"SELECT status, gitlab_note_id, posted_at FROM comments WHERE id = $1", commentID,
	).Scan(&status, &noteID, &postedAt); err != nil {
		t.Fatalf("read comment: %v", err)
	}
	if status != "posted" || noteID != 777 || postedAt == nil {
		t.Errorf("comment = (status %q, note %d, postedAt %v), want posted/777/non-nil", status, noteID, postedAt)
	}

	// API keys: store a hashed key and look it up by hash; unknown misses cleanly.
	apiKeys := NewAPIKeyRepo(db.Pool)
	if _, err := db.Pool.Exec(ctx, "TRUNCATE api_keys"); err != nil {
		t.Fatalf("truncate api_keys: %v", err)
	}
	keyRec, err := domain.NewAPIKey("ci-dashboard", "hash-abc")
	if err != nil {
		t.Fatalf("new api key: %v", err)
	}
	storedKey, err := apiKeys.Create(ctx, keyRec)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	if storedKey.ID == "" {
		t.Error("stored api key should carry an id")
	}
	foundKey, ok, err := apiKeys.FindByHash(ctx, "hash-abc")
	if err != nil || !ok {
		t.Fatalf("find api key = (ok %v, err %v), want (true, nil)", ok, err)
	}
	if foundKey.Name != "ci-dashboard" {
		t.Errorf("found key name = %q, want ci-dashboard", foundKey.Name)
	}
	if _, ok, err := apiKeys.FindByHash(ctx, "no-such-hash"); err != nil || ok {
		t.Errorf("FindByHash(unknown) = (ok %v, err %v), want (false, nil)", ok, err)
	}

	// Mark processed; the inbox should then be empty.
	if err := events.MarkProcessed(ctx, claimed.ID); err != nil {
		t.Fatalf("mark processed: %v", err)
	}
	if _, ok, err := events.ClaimNext(ctx); err != nil || ok {
		t.Errorf("ClaimNext after processed = (ok %v, err %v), want (false, nil)", ok, err)
	}
}
