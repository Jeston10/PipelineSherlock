// Package postgres (adapters) implements the application repository ports on
// top of a pgx connection pool. It depends on the application and domain
// layers, never the reverse.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jeston10/pipeline-sherlock/internal/application"
	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// uniqueViolation is the PostgreSQL SQLSTATE for a unique constraint violation.
const uniqueViolation = "23505"

// ProjectRepo implements application.ProjectRepository.
type ProjectRepo struct {
	pool *pgxpool.Pool
}

// NewProjectRepo constructs the repository over the given pool.
func NewProjectRepo(pool *pgxpool.Pool) *ProjectRepo {
	return &ProjectRepo{pool: pool}
}

// Ensure the adapter satisfies the port at compile time.
var _ application.ProjectRepository = (*ProjectRepo)(nil)

func (r *ProjectRepo) Create(ctx context.Context, p domain.Project) (domain.Project, error) {
	const q = `
INSERT INTO projects (gitlab_project_id, name, namespace, owner_id, webhook_secret_hash)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, created_at`

	var id string
	createdAt := p.CreatedAt
	row := r.pool.QueryRow(ctx, q, p.GitLabProjectID, p.Name, p.Namespace, p.OwnerID, p.WebhookSecretHash)
	if err := row.Scan(&id, &createdAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return domain.Project{}, application.ErrProjectAlreadyExists
		}
		return domain.Project{}, fmt.Errorf("insert project: %w", err)
	}
	return p.WithPersistence(domain.ProjectID(id), createdAt), nil
}

func (r *ProjectRepo) GetByGitLabID(ctx context.Context, gitlabProjectID int64) (domain.Project, bool, error) {
	const q = `
SELECT id, gitlab_project_id, name, namespace, owner_id, COALESCE(webhook_secret_hash, ''), created_at
FROM projects
WHERE gitlab_project_id = $1`

	var p domain.Project
	var id string
	row := r.pool.QueryRow(ctx, q, gitlabProjectID)
	if err := row.Scan(&id, &p.GitLabProjectID, &p.Name, &p.Namespace, &p.OwnerID, &p.WebhookSecretHash, &p.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Project{}, false, nil
		}
		return domain.Project{}, false, fmt.Errorf("select project: %w", err)
	}
	p.ID = domain.ProjectID(id)
	return p, true, nil
}

func (r *ProjectRepo) ListByOwner(ctx context.Context, ownerID string) ([]domain.Project, error) {
	const q = `
SELECT id, gitlab_project_id, name, namespace, owner_id, COALESCE(webhook_secret_hash, ''), created_at
FROM projects
WHERE owner_id = $1
ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, q, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var out []domain.Project
	for rows.Next() {
		var p domain.Project
		var id string
		if err := rows.Scan(&id, &p.GitLabProjectID, &p.Name, &p.Namespace, &p.OwnerID, &p.WebhookSecretHash, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		p.ID = domain.ProjectID(id)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projects: %w", err)
	}
	return out, nil
}
