package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jeston10/pipeline-sherlock/internal/application"
	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// WebhookEventRepo implements application.WebhookEventRepository.
type WebhookEventRepo struct {
	pool *pgxpool.Pool
}

// NewWebhookEventRepo constructs the repository over the given pool.
func NewWebhookEventRepo(pool *pgxpool.Pool) *WebhookEventRepo {
	return &WebhookEventRepo{pool: pool}
}

var _ application.WebhookEventRepository = (*WebhookEventRepo)(nil)

// Insert writes the event idempotently. The unique (project_id, delivery_key)
// constraint plus ON CONFLICT DO NOTHING means a redelivered webhook is a
// no-op; inserted reflects whether a row was actually written.
func (r *WebhookEventRepo) Insert(ctx context.Context, ev domain.WebhookEvent) (bool, error) {
	const q = `
INSERT INTO webhook_events (project_id, event_type, delivery_key, payload_json, status)
VALUES ($1, $2, $3, $4::jsonb, $5)
ON CONFLICT (project_id, delivery_key) DO NOTHING`

	tag, err := r.pool.Exec(ctx, q,
		string(ev.ProjectID),
		ev.EventType,
		ev.DeliveryKey,
		string(ev.Payload),
		string(ev.Status),
	)
	if err != nil {
		return false, fmt.Errorf("insert webhook event: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// ClaimNext atomically claims the oldest "received" event. The subquery takes a
// row lock with SKIP LOCKED so concurrent workers never claim the same row, and
// the outer UPDATE flips it to "processing" and bumps the attempt counter in the
// same statement. claimed is false when no event is waiting.
func (r *WebhookEventRepo) ClaimNext(ctx context.Context) (domain.WebhookEvent, bool, error) {
	const q = `
UPDATE webhook_events
SET status = 'processing', attempts = attempts + 1
WHERE id = (
    SELECT id FROM webhook_events
    WHERE status = 'received'
    ORDER BY received_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING id, project_id, event_type, delivery_key, payload_json, status, received_at, processed_at, COALESCE(error_code, ''), attempts`

	var (
		ev      domain.WebhookEvent
		id      string
		pid     string
		status  string
		payload []byte
	)
	row := r.pool.QueryRow(ctx, q)
	if err := row.Scan(&id, &pid, &ev.EventType, &ev.DeliveryKey, &payload,
		&status, &ev.ReceivedAt, &ev.ProcessedAt, &ev.ErrorCode, &ev.Attempts); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.WebhookEvent{}, false, nil
		}
		return domain.WebhookEvent{}, false, fmt.Errorf("claim next event: %w", err)
	}
	ev.ID = id
	ev.ProjectID = domain.ProjectID(pid)
	ev.Payload = payload
	ev.Status = domain.WebhookEventStatus(status)
	return ev, true, nil
}

// MarkProcessed records terminal success.
func (r *WebhookEventRepo) MarkProcessed(ctx context.Context, id string) error {
	const q = `UPDATE webhook_events SET status = 'processed', processed_at = now() WHERE id = $1`
	if _, err := r.pool.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("mark processed: %w", err)
	}
	return nil
}

// Requeue returns the event to "received" for another attempt.
func (r *WebhookEventRepo) Requeue(ctx context.Context, id, errorCode string) error {
	const q = `UPDATE webhook_events SET status = 'received', error_code = $2 WHERE id = $1`
	if _, err := r.pool.Exec(ctx, q, id, errorCode); err != nil {
		return fmt.Errorf("requeue event: %w", err)
	}
	return nil
}

// MarkFailed records terminal failure.
func (r *WebhookEventRepo) MarkFailed(ctx context.Context, id, errorCode string) error {
	const q = `UPDATE webhook_events SET status = 'failed', error_code = $2, processed_at = now() WHERE id = $1`
	if _, err := r.pool.Exec(ctx, q, id, errorCode); err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	return nil
}
