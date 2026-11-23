package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewPipeline(t *testing.T) {
	tests := []struct {
		name             string
		projectID        ProjectID
		gitlabPipelineID int64
		status           string
		wantErr          bool
	}{
		{"valid", "proj_1", 42, "failed", false},
		{"missing project id", "", 42, "failed", true},
		{"zero pipeline id", "proj_1", 0, "failed", true},
		{"negative pipeline id", "proj_1", -1, "failed", true},
		{"missing status", "proj_1", 42, "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewPipeline(tc.projectID, tc.gitlabPipelineID, "main", "abc123", tc.status)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, ErrInvalidPipeline) {
					t.Errorf("error = %v, want ErrInvalidPipeline", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.GitLabPipelineID != tc.gitlabPipelineID || p.Status != tc.status {
				t.Errorf("fields not set: %+v", p)
			}
		})
	}
}

func TestPipelineWithTiming_Immutable(t *testing.T) {
	p, err := NewPipeline("proj_1", 42, "main", "abc123", "success")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	started := time.Unix(1000, 0).UTC()
	finished := time.Unix(1090, 0).UTC()
	dur := 90

	updated := p.WithTiming(&started, &finished, &dur)

	if p.StartedAt != nil || p.FinishedAt != nil || p.DurationSeconds != nil {
		t.Errorf("WithTiming mutated the receiver: %+v", p)
	}
	if updated.StartedAt == nil || !updated.StartedAt.Equal(started) {
		t.Errorf("StartedAt = %v, want %v", updated.StartedAt, started)
	}
	if updated.DurationSeconds == nil || *updated.DurationSeconds != dur {
		t.Errorf("DurationSeconds = %v, want %d", updated.DurationSeconds, dur)
	}
}

func TestJobValid(t *testing.T) {
	tests := []struct {
		name string
		job  Job
		want bool
	}{
		{"valid", Job{GitLabJobID: 1, Name: "build", Status: "failed"}, true},
		{"zero id", Job{GitLabJobID: 0, Name: "build", Status: "failed"}, false},
		{"empty name", Job{GitLabJobID: 1, Name: "", Status: "failed"}, false},
		{"empty status", Job{GitLabJobID: 1, Name: "build", Status: ""}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.job.Valid(); got != tc.want {
				t.Errorf("Valid() = %v, want %v", got, tc.want)
			}
		})
	}
}
