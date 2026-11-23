// Package application holds use cases that orchestrate the domain. It depends
// on the domain and on small repository interfaces defined here (consumed-side
// interfaces); concrete implementations live in the adapter layer.
package application

import (
	"context"
	"errors"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// Repository-level sentinel errors.
var (
	// ErrProjectAlreadyExists is returned when registering a project whose
	// GitLab id is already registered.
	ErrProjectAlreadyExists = errors.New("project already exists")
	// ErrProjectNotFound is returned when no project matches a lookup.
	ErrProjectNotFound = errors.New("project not found")
)

// ProjectRepository persists and retrieves projects.
type ProjectRepository interface {
	// Create inserts a new project and returns it with its assigned ID and
	// CreatedAt. It returns ErrProjectAlreadyExists if the GitLab id is taken.
	Create(ctx context.Context, p domain.Project) (domain.Project, error)
	// GetByGitLabID returns the project with the given GitLab project id.
	// found is false (with a nil error) when no such project exists.
	GetByGitLabID(ctx context.Context, gitlabProjectID int64) (project domain.Project, found bool, err error)
	// List returns all registered projects.
	List(ctx context.Context) ([]domain.Project, error)
}

// WebhookEventRepository persists inbox events idempotently.
type WebhookEventRepository interface {
	// Insert stores the event. It is idempotent on (ProjectID, DeliveryKey):
	// inserted is false when an event with the same delivery key already
	// exists, in which case nothing is written. This is the de-duplication
	// mechanism for redelivered webhooks.
	Insert(ctx context.Context, ev domain.WebhookEvent) (inserted bool, err error)
}
