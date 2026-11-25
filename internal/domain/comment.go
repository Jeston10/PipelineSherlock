package domain

import (
	"errors"
	"fmt"
	"time"
)

// CommentStatus is the lifecycle state of a merge-request write-back comment.
type CommentStatus string

const (
	// CommentPending is a comment that has been recorded but not yet posted.
	CommentPending CommentStatus = "pending"
	// CommentPosted is a comment that was successfully posted to GitLab.
	CommentPosted CommentStatus = "posted"
	// CommentFailed is a comment whose post attempt failed.
	CommentFailed CommentStatus = "failed"
)

var (
	// ErrInvalidComment is the umbrella error for a comment that fails its
	// identity invariants.
	ErrInvalidComment = errors.New("invalid comment")

	// ErrEmptyCommentFingerprintID is returned when the fingerprint is missing.
	ErrEmptyCommentFingerprintID = fmt.Errorf("%w: fingerprint id required", ErrInvalidComment)
	// ErrNonPositiveMergeRequestIID is returned when the MR iid is not positive.
	ErrNonPositiveMergeRequestIID = fmt.Errorf("%w: merge request iid must be positive", ErrInvalidComment)
)

// Comment is a write-back note that PipelineSherlock posts to the merge request
// a failing pipeline belongs to. It is keyed by (fingerprint, merge request) so
// one incident yields at most one comment per MR; reprocessing is idempotent.
type Comment struct {
	ID              string
	FingerprintID   string
	MergeRequestIID int64
	GitLabNoteID    *int64
	Status          CommentStatus
	PostedAt        *time.Time
}

// NewComment builds a pending comment intent for a (fingerprint, MR) pair,
// validating the identity needed to persist and later post it.
func NewComment(fingerprintID string, mergeRequestIID int64) (Comment, error) {
	var problems []error
	if fingerprintID == "" {
		problems = append(problems, ErrEmptyCommentFingerprintID)
	}
	if mergeRequestIID <= 0 {
		problems = append(problems, ErrNonPositiveMergeRequestIID)
	}
	if len(problems) > 0 {
		return Comment{}, errors.Join(problems...)
	}
	return Comment{
		FingerprintID:   fingerprintID,
		MergeRequestIID: mergeRequestIID,
		Status:          CommentPending,
	}, nil
}

// WithPosted returns a copy marked posted, carrying the GitLab note id and the
// time it was posted. It never mutates the receiver.
func (c Comment) WithPosted(gitlabNoteID int64, at time.Time) Comment {
	noteID := gitlabNoteID
	postedAt := at
	c.Status = CommentPosted
	c.GitLabNoteID = &noteID
	c.PostedAt = &postedAt
	return c
}

// WithFailed returns a copy marked failed. It never mutates the receiver.
func (c Comment) WithFailed() Comment {
	c.Status = CommentFailed
	return c
}
