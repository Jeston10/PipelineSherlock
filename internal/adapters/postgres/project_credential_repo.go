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

// ProjectCredentialRepo implements application.ProjectCredentialRepository over
// the project_credentials table. It stores and returns ciphertext only; the
// plaintext token is sealed and opened in the application layer, never here.
type ProjectCredentialRepo struct {
	pool *pgxpool.Pool
}

// NewProjectCredentialRepo constructs the repository over the given pool.
func NewProjectCredentialRepo(pool *pgxpool.Pool) *ProjectCredentialRepo {
	return &ProjectCredentialRepo{pool: pool}
}

// Ensure the adapter satisfies the port at compile time.
var _ application.ProjectCredentialRepository = (*ProjectCredentialRepo)(nil)

// Upsert stores or replaces a project's encrypted token, refreshing updated_at.
func (r *ProjectCredentialRepo) Upsert(ctx context.Context, projectID domain.ProjectID, ciphertext []byte) error {
	const q = `
INSERT INTO project_credentials (project_id, token_ciphertext, updated_at)
VALUES ($1, $2, now())
ON CONFLICT (project_id)
DO UPDATE SET token_ciphertext = EXCLUDED.token_ciphertext, updated_at = now()`

	if _, err := r.pool.Exec(ctx, q, string(projectID), ciphertext); err != nil {
		return fmt.Errorf("upsert project credential: %w", err)
	}
	return nil
}

// Get returns the stored ciphertext for a project, or found=false when none.
func (r *ProjectCredentialRepo) Get(ctx context.Context, projectID domain.ProjectID) ([]byte, bool, error) {
	const q = `SELECT token_ciphertext FROM project_credentials WHERE project_id = $1`

	var ciphertext []byte
	row := r.pool.QueryRow(ctx, q, string(projectID))
	if err := row.Scan(&ciphertext); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("select project credential: %w", err)
	}
	return ciphertext, true, nil
}
