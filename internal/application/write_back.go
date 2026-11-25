package application

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// MergeRequestCommenter posts a note to a GitLab merge request. It is a small
// consumer-side interface (defined where it is used) so the write-back use case
// does not depend on the wider GitLabClient surface. The gitlab adapter
// satisfies both.
type MergeRequestCommenter interface {
	// PostMergeRequestNote posts body as a note on the merge request and returns
	// the created note's GitLab id.
	PostMergeRequestNote(ctx context.Context, gitlabProjectID, mergeRequestIID int64, body string) (noteID int64, err error)
}

// WriteBackInput is everything the write-back use case needs to record and post
// a single incident comment to a merge request.
type WriteBackInput struct {
	FingerprintID   string
	GitLabProjectID int64
	MergeRequestIID int64
	Category        string
	SignatureLine   string
	OccurrenceCount int64
	// IncidentURL is an optional deep link to the incident; omitted when empty.
	IncidentURL string
}

// WriteBackService records a merge-request comment intent and posts it to
// GitLab, idempotently: a second call for the same (fingerprint, MR) is a no-op
// because the repository enqueue is idempotent. The post is best-effort at the
// call site's discretion — a post failure marks the comment failed and returns
// the error so the caller can decide whether to retry.
type WriteBackService struct {
	comments CommentRepository
	gitlab   MergeRequestCommenter
	logger   *slog.Logger
}

// NewWriteBackService wires the service. A nil logger falls back to the default.
func NewWriteBackService(comments CommentRepository, gitlab MergeRequestCommenter, logger *slog.Logger) *WriteBackService {
	if logger == nil {
		logger = slog.Default()
	}
	return &WriteBackService{comments: comments, gitlab: gitlab, logger: logger}
}

// PostDiagnosis records a pending comment for the incident/MR and, if it was
// newly created, posts it to GitLab and marks it posted. posted is true only
// when a note was actually posted on this call; it is false (with a nil error)
// when the comment already existed (idempotent skip).
func (s *WriteBackService) PostDiagnosis(ctx context.Context, in WriteBackInput) (posted bool, err error) {
	comment, err := domain.NewComment(in.FingerprintID, in.MergeRequestIID)
	if err != nil {
		return false, fmt.Errorf("write-back: invalid comment: %w", err)
	}

	id, created, err := s.comments.Enqueue(ctx, comment)
	if err != nil {
		return false, fmt.Errorf("write-back: enqueue comment: %w", err)
	}
	if !created {
		s.logger.Info("write-back skipped; comment already recorded",
			slog.String("fingerprint_id", in.FingerprintID),
			slog.Int64("merge_request_iid", in.MergeRequestIID),
		)
		return false, nil
	}

	noteID, postErr := s.gitlab.PostMergeRequestNote(ctx, in.GitLabProjectID, in.MergeRequestIID, RenderIncidentComment(in))
	if postErr != nil {
		if markErr := s.comments.MarkFailed(ctx, id); markErr != nil {
			s.logger.Error("write-back: mark failed",
				slog.String("comment_id", id), slog.String("error", markErr.Error()))
		}
		return false, fmt.Errorf("write-back: post merge request note: %w", postErr)
	}

	if markErr := s.comments.MarkPosted(ctx, id, noteID); markErr != nil {
		return false, fmt.Errorf("write-back: mark posted: %w", markErr)
	}
	return true, nil
}

// RenderIncidentComment builds the deterministic Markdown body posted to the
// merge request. It is a pure function of its input: no clock, IO or
// randomness, so the same incident always renders the same comment.
func RenderIncidentComment(in WriteBackInput) string {
	var b strings.Builder
	b.WriteString("### PipelineSherlock: recurring CI failure detected\n\n")
	b.WriteString("A job in this merge request's pipeline failed with a signature that matches a known failure fingerprint.\n\n")
	fmt.Fprintf(&b, "- **Category:** `%s`\n", in.Category)
	fmt.Fprintf(&b, "- **Signature:** `%s`\n", in.SignatureLine)
	fmt.Fprintf(&b, "- **Occurrences recorded:** %d\n", in.OccurrenceCount)
	if in.IncidentURL != "" {
		fmt.Fprintf(&b, "\n[View incident history](%s)\n", in.IncidentURL)
	}
	b.WriteString("\n<sub>Posted automatically by PipelineSherlock. This is a deterministic fingerprint match, not a verdict on root cause.</sub>")
	return b.String()
}
