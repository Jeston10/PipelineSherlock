package postgres

import (
	"context"
	"fmt"

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
