// Package application holds use cases that orchestrate the domain. It depends
// on the domain and on small repository interfaces defined here (consumed-side
// interfaces); concrete implementations live in the adapter layer.
package application

import (
	"context"
	"errors"
	"time"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// Repository-level sentinel errors.
var (
	// ErrProjectAlreadyExists is returned when registering a project whose
	// GitLab id is already registered.
	ErrProjectAlreadyExists = errors.New("project already exists")
	// ErrProjectNotFound is returned when no project matches a lookup.
	ErrProjectNotFound = errors.New("project not found")
	// ErrIncidentNotFound is returned when no incident matches a fingerprint
	// within a project.
	ErrIncidentNotFound = errors.New("incident not found")
)

// ProjectRepository persists and retrieves projects.
type ProjectRepository interface {
	// Create inserts a new project and returns it with its assigned ID and
	// CreatedAt. It returns ErrProjectAlreadyExists if the GitLab id is taken.
	Create(ctx context.Context, p domain.Project) (domain.Project, error)
	// GetByGitLabID returns the project with the given GitLab project id,
	// including its OwnerID for authorization scoping. found is false (with a nil
	// error) when no such project exists.
	GetByGitLabID(ctx context.Context, gitlabProjectID int64) (project domain.Project, found bool, err error)
	// ListByOwner returns the registered projects owned by the given tenant.
	ListByOwner(ctx context.Context, ownerID string) ([]domain.Project, error)
}

// WebhookEventRepository persists inbox events idempotently and hands them out
// to workers for processing.
type WebhookEventRepository interface {
	// Insert stores the event. It is idempotent on (ProjectID, DeliveryKey):
	// inserted is false when an event with the same delivery key already
	// exists, in which case nothing is written. This is the de-duplication
	// mechanism for redelivered webhooks.
	Insert(ctx context.Context, ev domain.WebhookEvent) (inserted bool, err error)

	// ClaimNext atomically selects the oldest unprocessed event, marks it
	// "processing", increments its attempt counter, and returns it. It uses
	// row-level locking (FOR UPDATE SKIP LOCKED) so concurrent workers never
	// claim the same row. claimed is false (nil error) when the inbox is empty.
	ClaimNext(ctx context.Context) (ev domain.WebhookEvent, claimed bool, err error)

	// MarkProcessed records terminal success for the event.
	MarkProcessed(ctx context.Context, id string) error

	// Requeue returns the event to the "received" state for another attempt,
	// recording the last error code. Used while attempts remain.
	Requeue(ctx context.Context, id, errorCode string) error

	// MarkFailed records terminal failure for the event with an error code.
	MarkFailed(ctx context.Context, id, errorCode string) error
}

// PipelineRepository upserts reconciled pipeline and job state.
type PipelineRepository interface {
	// Upsert writes the pipeline and its jobs, replacing any prior state for
	// the same (project, gitlab pipeline id). It is idempotent so the worker
	// can safely reprocess an event. It returns a map from each job's GitLab id
	// to its persisted row id, so callers can attach failure occurrences.
	Upsert(ctx context.Context, p domain.Pipeline, jobs []domain.Job) (jobIDs map[int64]string, err error)
}

// FingerprintOccurrence is one observed instance of a fingerprinted failure,
// ready to be persisted.
type FingerprintOccurrence struct {
	ProjectID           domain.ProjectID
	JobID               string // the persisted jobs.id this failure was seen on
	Fingerprint         string
	Category            string
	NormalizedSignature string
	AlgorithmVersion    int
	Evidence            string
	EvidenceHash        string
}

// RecordOccurrenceResult reports what persisting an occurrence produced: the
// fingerprint row id, the running occurrence count, and whether this call wrote
// a genuinely new occurrence (false when it was a de-duplicated reprocess).
type RecordOccurrenceResult struct {
	FingerprintID   string
	OccurrenceCount int64
	NewOccurrence   bool
}

// FingerprintRepository persists deterministic failure fingerprints and their
// occurrences.
type FingerprintRepository interface {
	// RecordOccurrence upserts the fingerprint and appends an occurrence for the
	// job. It is idempotent per (job, fingerprint): recording the same job's
	// failure twice adds no second occurrence and does not double-count. The
	// result carries the fingerprint id, the running count, and whether a new
	// occurrence was written this call.
	RecordOccurrence(ctx context.Context, occ FingerprintOccurrence) (RecordOccurrenceResult, error)
}

// IncidentRepository reads grouped failure incidents.
type IncidentRepository interface {
	// ListByProject returns the project's incidents, most recently seen first,
	// bounded by limit.
	ListByProject(ctx context.Context, projectID domain.ProjectID, limit int) ([]domain.Incident, error)

	// GetByFingerprint returns a single incident header by its fingerprint
	// within the project. found is false (nil error) when no such incident
	// exists.
	GetByFingerprint(ctx context.Context, projectID domain.ProjectID, fingerprint string) (incident domain.Incident, found bool, err error)

	// ListOccurrences returns the enriched occurrence history for a fingerprint
	// (joined to its job and pipeline), most recent first, bounded by limit.
	ListOccurrences(ctx context.Context, fingerprintID string, limit int) ([]domain.IncidentOccurrence, error)
}

// CommentRepository persists merge-request write-back comments, keyed by
// (fingerprint, merge request) so one incident yields at most one comment per
// MR.
type CommentRepository interface {
	// Enqueue records a pending comment intent. It is idempotent on
	// (FingerprintID, MergeRequestIID): created is false when a comment for that
	// pair already exists, in which case nothing new is written. In both cases id
	// is the persisted row id, so the caller can update its status.
	Enqueue(ctx context.Context, c domain.Comment) (id string, created bool, err error)

	// MarkPosted records that the comment was posted, storing the GitLab note id
	// and stamping the posted time.
	MarkPosted(ctx context.Context, id string, gitlabNoteID int64) error

	// MarkFailed records that the comment's post attempt failed.
	MarkFailed(ctx context.Context, id string) error
}

// GitLabPipeline is the subset of a GitLab pipeline the worker reconciles.
type GitLabPipeline struct {
	ID         int64
	Ref        string
	SHA        string
	Status     string
	StartedAt  *time.Time
	FinishedAt *time.Time
	Duration   *int
}

// GitLabJob is the subset of a GitLab job the worker reconciles.
type GitLabJob struct {
	ID         int64
	Name       string
	Stage      string
	Status     string
	StartedAt  *time.Time
	FinishedAt *time.Time
	Duration   *int
}

// GitLabClient reads authoritative pipeline/job state from the GitLab API.
// Implementations handle pagination, timeouts and bounded retries internally.
type GitLabClient interface {
	GetPipeline(ctx context.Context, gitlabProjectID, pipelineID int64) (GitLabPipeline, error)
	ListPipelineJobs(ctx context.Context, gitlabProjectID, pipelineID int64) ([]GitLabJob, error)
	// GetJobTrace returns the raw log text for a job (used for fingerprinting).
	GetJobTrace(ctx context.Context, gitlabProjectID, jobID int64) (string, error)
}
