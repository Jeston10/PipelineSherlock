package http

import (
	"context"
	"log/slog"

	"github.com/Jeston10/pipeline-sherlock/internal/application"
	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// ProjectService is the subset of project use cases the HTTP layer needs.
// Defined here (consumer side) so the transport layer is not bound to a
// concrete service type.
type ProjectService interface {
	Register(ctx context.Context, ownerID string, gitlabProjectID int64, name, namespace string) (application.RegisterProjectResult, error)
	List(ctx context.Context, ownerID string) ([]domain.Project, error)
}

// WebhookIngestor is the subset of ingestion the HTTP layer needs.
type WebhookIngestor interface {
	Ingest(ctx context.Context, cmd application.IngestCommand) (application.IngestResult, error)
}

// IncidentLister is the subset of incident reads the HTTP layer needs.
type IncidentLister interface {
	List(ctx context.Context, ownerID string, gitlabProjectID int64, limit int) ([]domain.Incident, error)
	Detail(ctx context.Context, ownerID string, gitlabProjectID int64, fingerprint string, limit int) (domain.IncidentDetail, error)
}

// defaultMaxWebhookBytes bounds the size of an accepted webhook body.
const defaultMaxWebhookBytes int64 = 1 << 20 // 1 MiB

// Deps bundles everything the router needs to serve the API.
type Deps struct {
	Readiness       ReadinessChecker
	Projects        ProjectService
	Webhooks        WebhookIngestor
	Incidents       IncidentLister
	APIKeys         APIKeyAuthenticator
	Logger          *slog.Logger
	MaxWebhookBytes int64
}

func (d Deps) maxWebhookBytes() int64 {
	if d.MaxWebhookBytes > 0 {
		return d.MaxWebhookBytes
	}
	return defaultMaxWebhookBytes
}
