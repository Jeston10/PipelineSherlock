package postgres

import (
	"context"
	"fmt"

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
