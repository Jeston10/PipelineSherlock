package application

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// defaultMaxAttempts bounds how many times an event is retried before it is
// marked failed terminally.
const defaultMaxAttempts = 5

// ProcessEventsService drains the durable inbox: it claims an event, reconciles
// the pipeline/job state it refers to (against the GitLab API when a client is
// configured, otherwise from the webhook payload itself), upserts the result,
// and marks the event processed. Transient failures are requeued with the
// attempt counter until the bound is reached, then failed.
type ProcessEventsService struct {
	events      WebhookEventRepository
	pipelines   PipelineRepository
	gitlab      GitLabClient // nil when no GitLab token is configured
	logger      *slog.Logger
	maxAttempts int
}

// NewProcessEventsService wires the processor. gitlab may be nil, in which case
// reconciliation falls back to the webhook payload.
func NewProcessEventsService(events WebhookEventRepository, pipelines PipelineRepository, gitlab GitLabClient, logger *slog.Logger) *ProcessEventsService {
	if logger == nil {
		logger = slog.Default()
	}
	return &ProcessEventsService{
		events:      events,
		pipelines:   pipelines,
		gitlab:      gitlab,
		logger:      logger,
		maxAttempts: defaultMaxAttempts,
	}
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
	isPipeline, gitlabProjectID, glp, gljobs, err := parsePipelinePayload(ev.Payload)
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

	if err := s.pipelines.Upsert(ctx, pipeline, jobs); err != nil {
		return fmt.Errorf("upsert pipeline %d: %w", glp.ID, err)
	}
	return nil
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
