package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
	"github.com/Jeston10/pipeline-sherlock/internal/platform/token"
)

// Ingestion sentinel errors. These map to HTTP responses in the transport
// layer without leaking internal detail.
var (
	// ErrInvalidSignature means the supplied webhook token did not match the
	// project's stored secret.
	ErrInvalidSignature = errors.New("invalid webhook signature")
)

// IngestWebhookService validates and durably records inbound GitLab webhooks.
type IngestWebhookService struct {
	projects ProjectRepository
	events   WebhookEventRepository
}

// NewIngestWebhookService wires the use case with its repositories.
func NewIngestWebhookService(projects ProjectRepository, events WebhookEventRepository) *IngestWebhookService {
	return &IngestWebhookService{projects: projects, events: events}
}

// IngestCommand is the validated input to Ingest.
type IngestCommand struct {
	GitLabProjectID int64
	Token           string
	EventType       string
	DeliveryKey     string
	Payload         []byte
}

// IngestResult reports the outcome of a durable write.
type IngestResult struct {
	// Accepted is true when the event was newly stored; false when it was a
	// duplicate delivery (idempotent no-op). Both are success outcomes.
	Accepted bool
}

// Ingest authenticates the delivery against the project's webhook secret and,
// if valid, records it in the durable inbox. Redelivered events (same
// DeliveryKey) are de-duplicated and reported as not Accepted without error.
func (s *IngestWebhookService) Ingest(ctx context.Context, cmd IngestCommand) (IngestResult, error) {
	project, found, err := s.projects.GetByGitLabID(ctx, cmd.GitLabProjectID)
	if err != nil {
		return IngestResult{}, fmt.Errorf("look up project: %w", err)
	}
	if !found {
		return IngestResult{}, ErrProjectNotFound
	}

	if !token.Verify(project.WebhookSecretHash, cmd.Token) {
		return IngestResult{}, ErrInvalidSignature
	}

	event, err := domain.NewWebhookEvent(project.ID, cmd.EventType, cmd.DeliveryKey, cmd.Payload)
	if err != nil {
		return IngestResult{}, err
	}

	inserted, err := s.events.Insert(ctx, event)
	if err != nil {
		return IngestResult{}, fmt.Errorf("persist webhook event: %w", err)
	}

	return IngestResult{Accepted: inserted}, nil
}
