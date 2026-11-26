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
func (r *FingerprintRepo) RecordOccurrence(ctx context.Context, occ application.FingerprintOccurrence) (application.RecordOccurrenceResult, error) {
	var zero application.RecordOccurrenceResult

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return zero, fmt.Errorf("begin tx: %w", err)
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
		return zero, fmt.Errorf("upsert fingerprint: %w", err)
	}

	const occurrenceQ = `
INSERT INTO failure_occurrences (job_id, fingerprint_id, category, evidence_excerpt, evidence_hash)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (job_id, fingerprint_id) DO NOTHING`

	tag, err := tx.Exec(ctx, occurrenceQ,
		occ.JobID, fingerprintID, occ.Category, occ.Evidence, occ.EvidenceHash,
	)
	if err != nil {
		return zero, fmt.Errorf("insert occurrence: %w", err)
	}

	// Only count a genuinely new occurrence.
	newOccurrence := tag.RowsAffected() == 1
	var occurrenceCount int64
	if newOccurrence {
		const bumpQ = `UPDATE failure_fingerprints SET occurrence_count = occurrence_count + 1 WHERE id = $1 RETURNING occurrence_count`
		if err := tx.QueryRow(ctx, bumpQ, fingerprintID).Scan(&occurrenceCount); err != nil {
			return zero, fmt.Errorf("increment occurrence count: %w", err)
		}
	} else {
		const readQ = `SELECT occurrence_count FROM failure_fingerprints WHERE id = $1`
		if err := tx.QueryRow(ctx, readQ, fingerprintID).Scan(&occurrenceCount); err != nil {
			return zero, fmt.Errorf("read occurrence count: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return zero, fmt.Errorf("commit tx: %w", err)
	}
	return application.RecordOccurrenceResult{
		FingerprintID:   fingerprintID,
		OccurrenceCount: occurrenceCount,
		NewOccurrence:   newOccurrence,
	}, nil
}
