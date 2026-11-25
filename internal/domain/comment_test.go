package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewComment(t *testing.T) {
	tests := []struct {
		name            string
		fingerprintID   string
		mergeRequestIID int64
		wantErr         bool
	}{
		{
			name:            "valid pending comment",
			fingerprintID:   "fp-1",
			mergeRequestIID: 42,
			wantErr:         false,
		},
		{
			name:            "empty fingerprint id is rejected",
			fingerprintID:   "",
			mergeRequestIID: 42,
			wantErr:         true,
		},
		{
			name:            "non-positive merge request iid is rejected",
			fingerprintID:   "fp-1",
			mergeRequestIID: 0,
			wantErr:         true,
		},
		{
			name:            "negative merge request iid is rejected",
			fingerprintID:   "fp-1",
			mergeRequestIID: -3,
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewComment(tt.fingerprintID, tt.mergeRequestIID)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidComment) {
					t.Fatalf("error = %v, want ErrInvalidComment", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.FingerprintID != tt.fingerprintID {
				t.Errorf("FingerprintID = %q, want %q", c.FingerprintID, tt.fingerprintID)
			}
			if c.MergeRequestIID != tt.mergeRequestIID {
				t.Errorf("MergeRequestIID = %d, want %d", c.MergeRequestIID, tt.mergeRequestIID)
			}
			if c.Status != CommentPending {
				t.Errorf("Status = %q, want %q", c.Status, CommentPending)
			}
			if c.GitLabNoteID != nil {
				t.Errorf("GitLabNoteID = %v, want nil", c.GitLabNoteID)
			}
			if c.PostedAt != nil {
				t.Errorf("PostedAt = %v, want nil", c.PostedAt)
			}
		})
	}
}

func TestCommentWithPosted(t *testing.T) {
	base, err := NewComment("fp-1", 42)
	if err != nil {
		t.Fatalf("NewComment: %v", err)
	}
	at := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)

	posted := base.WithPosted(987, at)

	if posted.Status != CommentPosted {
		t.Errorf("Status = %q, want %q", posted.Status, CommentPosted)
	}
	if posted.GitLabNoteID == nil || *posted.GitLabNoteID != 987 {
		t.Errorf("GitLabNoteID = %v, want 987", posted.GitLabNoteID)
	}
	if posted.PostedAt == nil || !posted.PostedAt.Equal(at) {
		t.Errorf("PostedAt = %v, want %v", posted.PostedAt, at)
	}

	// Immutability: the receiver is unchanged.
	if base.Status != CommentPending {
		t.Errorf("base mutated: Status = %q, want %q", base.Status, CommentPending)
	}
	if base.GitLabNoteID != nil {
		t.Errorf("base mutated: GitLabNoteID = %v, want nil", base.GitLabNoteID)
	}
	if base.PostedAt != nil {
		t.Errorf("base mutated: PostedAt = %v, want nil", base.PostedAt)
	}
}

func TestCommentWithFailed(t *testing.T) {
	base, err := NewComment("fp-1", 42)
	if err != nil {
		t.Fatalf("NewComment: %v", err)
	}

	failed := base.WithFailed()

	if failed.Status != CommentFailed {
		t.Errorf("Status = %q, want %q", failed.Status, CommentFailed)
	}
	// Immutability: the receiver is unchanged.
	if base.Status != CommentPending {
		t.Errorf("base mutated: Status = %q, want %q", base.Status, CommentPending)
	}
}
