package application

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Jeston10/pipeline-sherlock/internal/analysis"
	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// failedJobStatus is the GitLab job status that triggers fingerprinting.
const failedJobStatus = "failed"

// defaultMaxAttempts bounds how many times an event is retried before it is
// marked failed terminally.
const defaultMaxAttempts = 5

// ProcessEventsService drains the durable inbox: it claims an event, reconciles
// the pipeline/job state it refers to (against the GitLab API when a client is
// configured, otherwise from the webhook payload itself), upserts the result,
// and marks the event processed. Transient failures are requeued with the
// attempt counter until the bound is reached, then failed.
type ProcessEventsService struct {
	events       WebhookEventRepository
	pipelines    PipelineRepository
	fingerprints FingerprintRepository
	gitlab       GitLabClient      // nil when no GitLab token is configured
	writeBack    *WriteBackService // nil when MR write-back is disabled
	logger       *slog.Logger
	maxAttempts  int
}

// ProcessOption configures optional processor behaviour.
type ProcessOption func(*ProcessEventsService)

// WithWriteBack enables posting an incident diagnosis back to the merge request
// a failing pipeline belongs to. When unset, write-back is skipped entirely.
func WithWriteBack(wb *WriteBackService) ProcessOption {
	return func(s *ProcessEventsService) { s.writeBack = wb }
}

// NewProcessEventsService wires the processor. gitlab may be nil, in which case
// reconciliation falls back to the webhook payload and failed jobs are not
// fingerprinted (traces are only available through the GitLab API). Optional
// behaviour such as merge-request write-back is supplied via options.
func NewProcessEventsService(events WebhookEventRepository, pipelines PipelineRepository, fingerprints FingerprintRepository, gitlab GitLabClient, logger *slog.Logger, opts ...ProcessOption) *ProcessEventsService {
	if logger == nil {
		logger = slog.Default()
	}
	s := &ProcessEventsService{
		events:       events,
		pipelines:    pipelines,
		fingerprints: fingerprints,
		gitlab:       gitlab,
		logger:       logger,
		maxAttempts:  defaultMaxAttempts,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ProcessNext claims and processes at most one event. progressed is true when
// an event was claimed (whether it succeeded, was requeued, or failed), and
// false when the inbox was empty. A returned error is infrastructural (claim or
// bookkeeping failed); per-event processing failures are handled in-band via
// requeue/fail and do not stop the worker.
func (s *ProcessEventsService) ProcessNext(ctx context.Context) (progressed bool, err error) {
	ev, claimed, err := s.events.ClaimNext(ctx)
	if err != nil {
		return false, fmt.Errorf("claim next event: %w", err)
	}
	if !claimed {
		return false, nil
	}

	if procErr := s.reconcile(ctx, ev); procErr != nil {
		return true, s.handleFailure(ctx, ev, procErr)
	}

	if err := s.events.MarkProcessed(ctx, ev.ID); err != nil {
		return true, fmt.Errorf("mark processed %s: %w", ev.ID, err)
	}
	return true, nil
}

// reconcile turns one inbox event into persisted pipeline/job rows.
func (s *ProcessEventsService) reconcile(ctx context.Context, ev domain.WebhookEvent) error {
	isPipeline, gitlabProjectID, mergeRequestIID, glp, gljobs, err := parsePipelinePayload(ev.Payload)
	if err != nil {
		return fmt.Errorf("parse payload: %w", err)
	}
	if !isPipeline {
		// Nothing to reconcile for non-pipeline events; treat as processed.
		return nil
	}

	// Prefer authoritative GitLab state when a client is configured; the webhook
	// payload can be stale by the time we process it.
	if s.gitlab != nil {
		fetched, ferr := s.gitlab.GetPipeline(ctx, gitlabProjectID, glp.ID)
		if ferr != nil {
			return fmt.Errorf("fetch pipeline %d: %w", glp.ID, ferr)
		}
		glp = fetched
		jobs, jerr := s.gitlab.ListPipelineJobs(ctx, gitlabProjectID, glp.ID)
		if jerr != nil {
			return fmt.Errorf("list jobs for pipeline %d: %w", glp.ID, jerr)
		}
		gljobs = jobs
	}

	pipeline, err := domain.NewPipeline(ev.ProjectID, glp.ID, glp.Ref, glp.SHA, glp.Status)
	if err != nil {
		return fmt.Errorf("build pipeline: %w", err)
	}
	pipeline = pipeline.WithTiming(glp.StartedAt, glp.FinishedAt, glp.Duration)

	jobs := make([]domain.Job, 0, len(gljobs))
	for _, gj := range gljobs {
		job := domain.Job{
			GitLabJobID:     gj.ID,
			Name:            gj.Name,
			Stage:           gj.Stage,
			Status:          gj.Status,
			StartedAt:       gj.StartedAt,
			FinishedAt:      gj.FinishedAt,
			DurationSeconds: gj.Duration,
		}
		if !job.Valid() {
			continue // skip incomplete job entries rather than failing the batch
		}
		jobs = append(jobs, job)
	}

	jobIDs, err := s.pipelines.Upsert(ctx, pipeline, jobs)
	if err != nil {
		return fmt.Errorf("upsert pipeline %d: %w", glp.ID, err)
	}

	// Fingerprint failed jobs. This needs job traces, which are only available
	// through the GitLab API, so it runs only when a client is configured.
	if s.gitlab != nil && s.fingerprints != nil {
		s.fingerprintFailedJobs(ctx, ev.ProjectID, gitlabProjectID, mergeRequestIID, gljobs, jobIDs)
	}
	return nil
}

// fingerprintFailedJobs analyses each failed job's trace and records the
// resulting diagnosis. It is best-effort: a per-job failure (trace fetch,
// persistence) is logged and skipped rather than failing the whole event, since
// the pipeline/job state has already been persisted.
func (s *ProcessEventsService) fingerprintFailedJobs(ctx context.Context, projectID domain.ProjectID, gitlabProjectID, mergeRequestIID int64, gljobs []GitLabJob, jobIDs map[int64]string) {
	for _, gj := range gljobs {
		if gj.Status != failedJobStatus {
			continue
		}
		jobID, ok := jobIDs[gj.ID]
		if !ok {
			continue
		}

		trace, err := s.gitlab.GetJobTrace(ctx, gitlabProjectID, gj.ID)
		if err != nil {
			s.logger.Warn("fetch job trace failed",
				slog.Int64("gitlab_job_id", gj.ID), slog.String("error", err.Error()))
			continue
		}

		diag := analysis.Analyze(string(projectID), trace)
		occ := FingerprintOccurrence{
			ProjectID:           projectID,
			JobID:               jobID,
			Fingerprint:         diag.Fingerprint,
			Category:            string(diag.Category),
			NormalizedSignature: diag.NormalizedSignature,
			AlgorithmVersion:    diag.AlgorithmVersion,
			Evidence:            diag.Evidence,
			EvidenceHash:        diag.EvidenceHash,
		}
		result, err := s.fingerprints.RecordOccurrence(ctx, occ)
		if err != nil {
			s.logger.Warn("record occurrence failed",
				slog.Int64("gitlab_job_id", gj.ID), slog.String("error", err.Error()))
			continue
		}

		s.maybeWriteBack(ctx, gitlabProjectID, mergeRequestIID, diag, result)
	}
}

// maybeWriteBack posts the incident diagnosis to the merge request when
// write-back is enabled, the pipeline is attached to an MR, and this call
// recorded a new occurrence. It is best-effort: a post failure is logged (and
// recorded as a failed comment inside the service), never failing the event.
func (s *ProcessEventsService) maybeWriteBack(ctx context.Context, gitlabProjectID, mergeRequestIID int64, diag analysis.Diagnosis, result RecordOccurrenceResult) {
	if s.writeBack == nil || mergeRequestIID <= 0 || !result.NewOccurrence {
		return
	}
	_, err := s.writeBack.PostDiagnosis(ctx, WriteBackInput{
		FingerprintID:   result.FingerprintID,
		GitLabProjectID: gitlabProjectID,
		MergeRequestIID: mergeRequestIID,
		Category:        string(diag.Category),
		SignatureLine:   diag.NormalizedSignature,
		OccurrenceCount: result.OccurrenceCount,
	})
	if err != nil {
		s.logger.Warn("merge-request write-back failed",
			slog.Int64("merge_request_iid", mergeRequestIID), slog.String("error", err.Error()))
	}
}

// handleFailure requeues a transient failure until the attempt bound is reached,
// then fails the event terminally. ev.Attempts already reflects this attempt.
func (s *ProcessEventsService) handleFailure(ctx context.Context, ev domain.WebhookEvent, cause error) error {
	code := "processing_error"
	if ev.Attempts >= s.maxAttempts {
		s.logger.Error("event failed terminally",
			slog.String("event_id", ev.ID),
			slog.Int("attempts", ev.Attempts),
			slog.String("error", cause.Error()),
		)
		if err := s.events.MarkFailed(ctx, ev.ID, code); err != nil {
			return fmt.Errorf("mark failed %s: %w", ev.ID, err)
		}
		return nil
	}

	s.logger.Warn("event requeued after transient failure",
		slog.String("event_id", ev.ID),
		slog.Int("attempts", ev.Attempts),
		slog.String("error", cause.Error()),
	)
	if err := s.events.Requeue(ctx, ev.ID, code); err != nil {
		return fmt.Errorf("requeue %s: %w", ev.ID, err)
	}
	return nil
}
