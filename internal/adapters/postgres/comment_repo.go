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

// CommentRepo implements application.CommentRepository over the comments table.
type CommentRepo struct {
	pool *pgxpool.Pool
}

// NewCommentRepo constructs the repository over the given pool.
func NewCommentRepo(pool *pgxpool.Pool) *CommentRepo {
	return &CommentRepo{pool: pool}
}

var _ application.CommentRepository = (*CommentRepo)(nil)

// Enqueue records a pending comment intent. It is idempotent on
// (fingerprint_id, merge_request_iid): the INSERT ... ON CONFLICT DO NOTHING
// returns the new id (created = true) on first write; on a conflict it returns
// no row, so the existing row's id is loaded and created is false.
func (r *CommentRepo) Enqueue(ctx context.Context, c domain.Comment) (string, bool, error) {
	const ins = `
INSERT INTO comments (fingerprint_id, merge_request_iid, status)
VALUES ($1, $2, $3)
ON CONFLICT (fingerprint_id, merge_request_iid) DO NOTHING
RETURNING id`

	var id string
	err := r.pool.QueryRow(ctx, ins, c.FingerprintID, c.MergeRequestIID, string(c.Status)).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("enqueue comment: %w", err)
	}

	// Conflict: a comment already exists for this (fingerprint, MR).
	const sel = `SELECT id FROM comments WHERE fingerprint_id = $1 AND merge_request_iid = $2`
	if err := r.pool.QueryRow(ctx, sel, c.FingerprintID, c.MergeRequestIID).Scan(&id); err != nil {
		return "", false, fmt.Errorf("load existing comment: %w", err)
	}
	return id, false, nil
}

// MarkPosted records that the comment was posted, storing the GitLab note id and
// stamping posted_at. It errors if no comment has the given id.
func (r *CommentRepo) MarkPosted(ctx context.Context, id string, gitlabNoteID int64) error {
	const q = `UPDATE comments SET status = 'posted', gitlab_note_id = $2, posted_at = now() WHERE id = $1`
	ct, err := r.pool.Exec(ctx, q, id, gitlabNoteID)
	if err != nil {
		return fmt.Errorf("mark comment posted: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("mark comment posted: no comment with id %s", id)
	}
	return nil
}

// MarkFailed records that the comment's post attempt failed. It errors if no
// comment has the given id.
func (r *CommentRepo) MarkFailed(ctx context.Context, id string) error {
	const q = `UPDATE comments SET status = 'failed' WHERE id = $1`
	ct, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("mark comment failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("mark comment failed: no comment with id %s", id)
	}
	return nil
}
