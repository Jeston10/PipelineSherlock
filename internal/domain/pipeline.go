package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidPipeline is returned when a Pipeline fails its identity invariants.
var ErrInvalidPipeline = errors.New("invalid pipeline")

// Pipeline is a GitLab CI pipeline run belonging to a registered project.
// It is a projection of GitLab state, so most timestamp fields are optional.
type Pipeline struct {
	ProjectID        ProjectID
	GitLabPipelineID int64
	Ref              string
	SHA              string
	Status           string
	StartedAt        *time.Time
	FinishedAt       *time.Time
	DurationSeconds  *int
}

// Job is a single job within a pipeline.
type Job struct {
	GitLabJobID     int64
	Name            string
	Stage           string
	Status          string
	StartedAt       *time.Time
	FinishedAt      *time.Time
	DurationSeconds *int
}

// NewPipeline validates the minimum identity needed to persist a pipeline.
func NewPipeline(projectID ProjectID, gitlabPipelineID int64, ref, sha, status string) (Pipeline, error) {
	var problems []error
	if projectID == "" {
		problems = append(problems, fmt.Errorf("%w: project id required", ErrInvalidPipeline))
	}
	if gitlabPipelineID <= 0 {
		problems = append(problems, fmt.Errorf("%w: gitlab pipeline id must be positive", ErrInvalidPipeline))
	}
	if status == "" {
		problems = append(problems, fmt.Errorf("%w: status required", ErrInvalidPipeline))
	}
	if len(problems) > 0 {
		return Pipeline{}, errors.Join(problems...)
	}
	return Pipeline{
		ProjectID:        projectID,
		GitLabPipelineID: gitlabPipelineID,
		Ref:              ref,
		SHA:              sha,
		Status:           status,
	}, nil
}

// WithTiming returns a copy with started/finished timestamps and an optional
// duration applied. It never mutates the receiver.
func (p Pipeline) WithTiming(startedAt, finishedAt *time.Time, durationSeconds *int) Pipeline {
	p.StartedAt = startedAt
	p.FinishedAt = finishedAt
	p.DurationSeconds = durationSeconds
	return p
}

// Valid reports whether a job carries the minimum identity to be persisted.
func (j Job) Valid() bool {
	return j.GitLabJobID > 0 && j.Name != "" && j.Status != ""
}
