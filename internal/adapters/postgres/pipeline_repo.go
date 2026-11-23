package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jeston10/pipeline-sherlock/internal/application"
	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// PipelineRepo implements application.PipelineRepository.
type PipelineRepo struct {
	pool *pgxpool.Pool
}

// NewPipelineRepo constructs the repository over the given pool.
func NewPipelineRepo(pool *pgxpool.Pool) *PipelineRepo {
	return &PipelineRepo{pool: pool}
}

var _ application.PipelineRepository = (*PipelineRepo)(nil)

// Upsert writes the pipeline and its jobs in a single transaction. Both use
// ON CONFLICT DO UPDATE keyed on their GitLab ids, so reprocessing the same
// event converges to the latest state rather than duplicating rows. It returns
// a map from each job's GitLab id to its persisted jobs.id.
func (r *PipelineRepo) Upsert(ctx context.Context, p domain.Pipeline, jobs []domain.Job) (map[int64]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after a successful commit is a no-op

	const pipelineQ = `
INSERT INTO pipelines (project_id, gitlab_pipeline_id, ref, sha, status, started_at, finished_at, duration_seconds)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (project_id, gitlab_pipeline_id) DO UPDATE
SET ref = EXCLUDED.ref,
    sha = EXCLUDED.sha,
    status = EXCLUDED.status,
    started_at = EXCLUDED.started_at,
    finished_at = EXCLUDED.finished_at,
    duration_seconds = EXCLUDED.duration_seconds
RETURNING id`

	var pipelineID string
	row := tx.QueryRow(ctx, pipelineQ,
		string(p.ProjectID), p.GitLabPipelineID, p.Ref, p.SHA, p.Status,
		p.StartedAt, p.FinishedAt, p.DurationSeconds,
	)
	if err := row.Scan(&pipelineID); err != nil {
		return nil, fmt.Errorf("upsert pipeline: %w", err)
	}

	const jobQ = `
INSERT INTO jobs (pipeline_id, gitlab_job_id, name, stage, status, started_at, finished_at, duration_seconds)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (pipeline_id, gitlab_job_id) DO UPDATE
SET name = EXCLUDED.name,
    stage = EXCLUDED.stage,
    status = EXCLUDED.status,
    started_at = EXCLUDED.started_at,
    finished_at = EXCLUDED.finished_at,
    duration_seconds = EXCLUDED.duration_seconds
RETURNING id`

	jobIDs := make(map[int64]string, len(jobs))
	for _, j := range jobs {
		var jobID string
		if err := tx.QueryRow(ctx, jobQ,
			pipelineID, j.GitLabJobID, j.Name, j.Stage, j.Status,
			j.StartedAt, j.FinishedAt, j.DurationSeconds,
		).Scan(&jobID); err != nil {
			return nil, fmt.Errorf("upsert job %d: %w", j.GitLabJobID, err)
		}
		jobIDs[j.GitLabJobID] = jobID
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return jobIDs, nil
}
