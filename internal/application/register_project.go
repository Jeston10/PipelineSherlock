package application

import (
	"context"
	"fmt"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
	"github.com/Jeston10/pipeline-sherlock/internal/platform/token"
)

// RegisterProjectService registers GitLab projects for ingestion.
type RegisterProjectService struct {
	projects ProjectRepository
}

// NewRegisterProjectService wires the use case with its repository.
func NewRegisterProjectService(projects ProjectRepository) *RegisterProjectService {
	return &RegisterProjectService{projects: projects}
}

// RegisterProjectResult carries the persisted project and the plaintext webhook
// token. The token is returned exactly once at registration and never stored or
// retrievable afterwards; only its hash is persisted.
type RegisterProjectResult struct {
	Project       domain.Project
	WebhookSecret string
}

// Register validates the inputs, generates a webhook secret, persists the
// project with only the secret's hash, and returns the project plus the
// plaintext secret for one-time display.
func (s *RegisterProjectService) Register(ctx context.Context, gitlabProjectID int64, name, namespace string) (RegisterProjectResult, error) {
	project, err := domain.NewProject(gitlabProjectID, name, namespace)
	if err != nil {
		return RegisterProjectResult{}, err
	}

	secret, err := token.Generate()
	if err != nil {
		return RegisterProjectResult{}, fmt.Errorf("generate webhook secret: %w", err)
	}
	project = project.WithWebhookSecretHash(token.Hash(secret))

	stored, err := s.projects.Create(ctx, project)
	if err != nil {
		return RegisterProjectResult{}, fmt.Errorf("persist project: %w", err)
	}

	return RegisterProjectResult{Project: stored, WebhookSecret: secret}, nil
}

// List returns all registered projects.
func (s *RegisterProjectService) List(ctx context.Context) ([]domain.Project, error) {
	return s.projects.List(ctx)
}
