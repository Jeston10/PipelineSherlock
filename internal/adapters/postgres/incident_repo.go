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

// IncidentRepo implements application.IncidentRepository.
type IncidentRepo struct {
	pool *pgxpool.Pool
}

// NewIncidentRepo constructs the repository over the given pool.
func NewIncidentRepo(pool *pgxpool.Pool) *IncidentRepo {
	return &IncidentRepo{pool: pool}
}

var _ application.IncidentRepository = (*IncidentRepo)(nil)

// ListByProject returns a project's incidents (one row per fingerprint),
// most recently seen first.
func (r *IncidentRepo) ListByProject(ctx context.Context, projectID domain.ProjectID, limit int) ([]domain.Incident, error) {
	const q = `
SELECT id, project_id, fingerprint, category, normalized_signature, algorithm_version,
       occurrence_count, first_seen_at, last_seen_at
FROM failure_fingerprints
WHERE project_id = $1
ORDER BY last_seen_at DESC
LIMIT $2`

	rows, err := r.pool.Query(ctx, q, string(projectID), limit)
	if err != nil {
		return nil, fmt.Errorf("list incidents: %w", err)
	}
	defer rows.Close()

	var out []domain.Incident
	for rows.Next() {
		var inc domain.Incident
		var pid string
		if err := rows.Scan(
			&inc.FingerprintID, &pid, &inc.Fingerprint, &inc.Category, &inc.NormalizedSignature,
			&inc.AlgorithmVersion, &inc.OccurrenceCount, &inc.FirstSeenAt, &inc.LastSeenAt,
		); err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}
		inc.ProjectID = domain.ProjectID(pid)
		out = append(out, inc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incidents: %w", err)
	}
	return out, nil
}

// GetByFingerprint returns a single incident header by its fingerprint within
// the project. When more than one algorithm version shares a fingerprint
// string (not expected, since the version is part of the hash input), the most
// recently seen row wins. found is false (nil error) when no row matches.
func (r *IncidentRepo) GetByFingerprint(ctx context.Context, projectID domain.ProjectID, fingerprint string) (domain.Incident, bool, error) {
	const q = `
SELECT id, project_id, fingerprint, category, normalized_signature, algorithm_version,
       occurrence_count, first_seen_at, last_seen_at
FROM failure_fingerprints
WHERE project_id = $1 AND fingerprint = $2
ORDER BY last_seen_at DESC
LIMIT 1`

	var inc domain.Incident
	var pid string
	err := r.pool.QueryRow(ctx, q, string(projectID), fingerprint).Scan(
		&inc.FingerprintID, &pid, &inc.Fingerprint, &inc.Category, &inc.NormalizedSignature,
		&inc.AlgorithmVersion, &inc.OccurrenceCount, &inc.FirstSeenAt, &inc.LastSeenAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Incident{}, false, nil
	}
	if err != nil {
		return domain.Incident{}, false, fmt.Errorf("get incident by fingerprint: %w", err)
	}
	inc.ProjectID = domain.ProjectID(pid)
	return inc, true, nil
}

// ListOccurrences returns the enriched occurrence history for a fingerprint,
// joining each failure_occurrences row to its job and pipeline for ref, sha,
// stage and timing context. Most recent first, bounded by limit.
func (r *IncidentRepo) ListOccurrences(ctx context.Context, fingerprintID string, limit int) ([]domain.IncidentOccurrence, error) {
	const q = `
SELECT o.observed_at, p.ref, p.sha, j.name, j.stage, j.gitlab_job_id,
       p.gitlab_pipeline_id, o.evidence_excerpt
FROM failure_occurrences o
JOIN jobs j ON j.id = o.job_id
JOIN pipelines p ON p.id = j.pipeline_id
WHERE o.fingerprint_id = $1
ORDER BY o.observed_at DESC
LIMIT $2`

	rows, err := r.pool.Query(ctx, q, fingerprintID, limit)
	if err != nil {
		return nil, fmt.Errorf("list occurrences: %w", err)
	}
	defer rows.Close()

	var out []domain.IncidentOccurrence
	for rows.Next() {
		var o domain.IncidentOccurrence
		if err := rows.Scan(
			&o.ObservedAt, &o.Ref, &o.SHA, &o.JobName, &o.Stage,
			&o.GitLabJobID, &o.GitLabPipelineID, &o.EvidenceExcerpt,
		); err != nil {
			return nil, fmt.Errorf("scan occurrence: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate occurrences: %w", err)
	}
	return out, nil
}
