package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jeston10/pipeline-sherlock/internal/application"
)

// FingerprintRepo implements application.FingerprintRepository.
type FingerprintRepo struct {
	pool *pgxpool.Pool
}

// NewFingerprintRepo constructs the repository over the given pool.
func NewFingerprintRepo(pool *pgxpool.Pool) *FingerprintRepo {
	return &FingerprintRepo{pool: pool}
}

var _ application.FingerprintRepository = (*FingerprintRepo)(nil)

// RecordOccurrence upserts the fingerprint and appends an occurrence for the
// job, all in one transaction. The occurrence insert is idempotent on
// (job_id, fingerprint_id); occurrence_count is incremented only when a new
// occurrence row is actually written, so reprocessing never double-counts.
func (r *FingerprintRepo) RecordOccurrence(ctx context.Context, occ application.FingerprintOccurrence) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after a successful commit is a no-op

	const fingerprintQ = `
INSERT INTO failure_fingerprints (project_id, fingerprint, category, normalized_signature, algorithm_version)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (project_id, fingerprint, algorithm_version) DO UPDATE
SET last_seen_at = now()
RETURNING id`

	var fingerprintID string
	if err := tx.QueryRow(ctx, fingerprintQ,
		string(occ.ProjectID), occ.Fingerprint, occ.Category, occ.NormalizedSignature, occ.AlgorithmVersion,
	).Scan(&fingerprintID); err != nil {
		return fmt.Errorf("upsert fingerprint: %w", err)
	}

	const occurrenceQ = `
INSERT INTO failure_occurrences (job_id, fingerprint_id, category, evidence_excerpt, evidence_hash)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (job_id, fingerprint_id) DO NOTHING`

	tag, err := tx.Exec(ctx, occurrenceQ,
		occ.JobID, fingerprintID, occ.Category, occ.Evidence, occ.EvidenceHash,
	)
	if err != nil {
		return fmt.Errorf("insert occurrence: %w", err)
	}

	// Only count a genuinely new occurrence.
	if tag.RowsAffected() == 1 {
		const bumpQ = `UPDATE failure_fingerprints SET occurrence_count = occurrence_count + 1 WHERE id = $1`
		if _, err := tx.Exec(ctx, bumpQ, fingerprintID); err != nil {
			return fmt.Errorf("increment occurrence count: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
