package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jeston10/pipeline-sherlock/internal/application"
	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// APIKeyRepo implements application.APIKeyRepository over the api_keys table.
type APIKeyRepo struct {
	pool *pgxpool.Pool
}

// NewAPIKeyRepo constructs the repository over the given pool.
func NewAPIKeyRepo(pool *pgxpool.Pool) *APIKeyRepo {
	return &APIKeyRepo{pool: pool}
}

var _ application.APIKeyRepository = (*APIKeyRepo)(nil)

// Create inserts a new API key (hash only) and returns it with its assigned id
// and created_at.
func (r *APIKeyRepo) Create(ctx context.Context, k domain.APIKey) (domain.APIKey, error) {
	const q = `
INSERT INTO api_keys (name, owner_id, key_hash)
VALUES ($1, $2, $3)
RETURNING id, created_at`

	var id string
	createdAt := k.CreatedAt
	row := r.pool.QueryRow(ctx, q, k.Name, k.OwnerID, k.KeyHash)
	if err := row.Scan(&id, &createdAt); err != nil {
		return domain.APIKey{}, fmt.Errorf("insert api key: %w", err)
	}
	k.ID = id
	k.CreatedAt = createdAt
	return k, nil
}

// FindByHash returns the key whose stored hash equals keyHash. found is false
// (nil error) when no key matches.
func (r *APIKeyRepo) FindByHash(ctx context.Context, keyHash string) (domain.APIKey, bool, error) {
	const q = `
SELECT id, name, owner_id, key_hash, created_at, last_used_at
FROM api_keys
WHERE key_hash = $1`

	var k domain.APIKey
	var id string
	var lastUsed *time.Time
	row := r.pool.QueryRow(ctx, q, keyHash)
	if err := row.Scan(&id, &k.Name, &k.OwnerID, &k.KeyHash, &k.CreatedAt, &lastUsed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.APIKey{}, false, nil
		}
		return domain.APIKey{}, false, fmt.Errorf("select api key: %w", err)
	}
	k.ID = id
	k.LastUsedAt = lastUsed
	return k, true, nil
}
