package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// Per-project credential errors.
var (
	// ErrEncryptionKeyRequired is returned when an operation needs the master
	// encryption key but none is configured (no ENCRYPTION_KEY). Storing or
	// decrypting a per-project token cannot proceed without it.
	ErrEncryptionKeyRequired = errors.New("encryption key required")
	// ErrNoGitLabToken signals that no GitLab token is available for a project:
	// it has no stored credential and no global token is configured. The GitLab
	// adapter returns it so the worker falls back to the webhook payload instead
	// of failing the event.
	ErrNoGitLabToken = errors.New("no gitlab token for project")
	// ErrEmptyToken is returned when a blank token is offered for storage.
	ErrEmptyToken = errors.New("gitlab token must not be empty")
)

// TokenSealer seals and opens secret values for storage at rest. The crypto
// platform package provides the implementation (envelope encryption).
type TokenSealer interface {
	Seal(plaintext []byte) ([]byte, error)
	Open(ciphertext []byte) ([]byte, error)
}

// ProjectCredentialRepository persists each project's encrypted GitLab token.
// The repository stores and returns ciphertext only; it never sees plaintext.
type ProjectCredentialRepository interface {
	// Upsert stores (or replaces) the ciphertext for a project.
	Upsert(ctx context.Context, projectID domain.ProjectID, ciphertext []byte) error
	// Get returns the stored ciphertext for a project. found is false (nil error)
	// when the project has no stored credential.
	Get(ctx context.Context, projectID domain.ProjectID) (ciphertext []byte, found bool, err error)
}

// SetProjectTokenService stores a project's GitLab token encrypted at rest. The
// caller's owner must match the project's owner, so a tenant can only set its
// own projects' tokens; a cross-owner project reads as not found (no existence
// leak), exactly as for reads.
type SetProjectTokenService struct {
	projects ProjectRepository
	creds    ProjectCredentialRepository
	sealer   TokenSealer
}

// NewSetProjectTokenService wires the service. sealer must be non-nil: storing a
// token requires the encryption key.
func NewSetProjectTokenService(projects ProjectRepository, creds ProjectCredentialRepository, sealer TokenSealer) *SetProjectTokenService {
	return &SetProjectTokenService{projects: projects, creds: creds, sealer: sealer}
}

// Set encrypts and stores the token for the owner's project identified by its
// GitLab id.
func (s *SetProjectTokenService) Set(ctx context.Context, ownerID string, gitlabProjectID int64, token string) error {
	if s.sealer == nil {
		return ErrEncryptionKeyRequired
	}
	if strings.TrimSpace(token) == "" {
		return ErrEmptyToken
	}

	project, found, err := s.projects.GetByGitLabID(ctx, gitlabProjectID)
	if err != nil {
		return fmt.Errorf("look up project: %w", err)
	}
	// A cross-owner or unknown project is reported as not found so existence
	// never leaks across tenants.
	if !found || project.OwnerID != ownerID {
		return ErrProjectNotFound
	}

	ciphertext, err := s.sealer.Seal([]byte(token))
	if err != nil {
		return fmt.Errorf("seal token: %w", err)
	}
	if err := s.creds.Upsert(ctx, project.ID, ciphertext); err != nil {
		return fmt.Errorf("store credential: %w", err)
	}
	return nil
}

// ProjectTokenResolver resolves the GitLab token to use for a project: the
// project's own stored token when present, otherwise the global token when one
// is configured. It implements the GitLab adapter's token-resolver contract
// (keyed on the GitLab project id, which is what the adapter has at call time).
type ProjectTokenResolver struct {
	projects    ProjectRepository
	creds       ProjectCredentialRepository
	sealer      TokenSealer // may be nil when no encryption key is configured
	globalToken string      // may be empty
}

// NewProjectTokenResolver wires the resolver. sealer may be nil (then only the
// global token is available); globalToken may be empty.
func NewProjectTokenResolver(projects ProjectRepository, creds ProjectCredentialRepository, sealer TokenSealer, globalToken string) *ProjectTokenResolver {
	return &ProjectTokenResolver{projects: projects, creds: creds, sealer: sealer, globalToken: strings.TrimSpace(globalToken)}
}

// TokenForGitLabProject returns the token for the given GitLab project id. It
// returns an empty string (nil error) when neither a per-project credential nor
// a global token is available; the adapter turns that into ErrNoGitLabToken.
func (r *ProjectTokenResolver) TokenForGitLabProject(ctx context.Context, gitlabProjectID int64) (string, error) {
	project, found, err := r.projects.GetByGitLabID(ctx, gitlabProjectID)
	if err != nil {
		return "", fmt.Errorf("look up project: %w", err)
	}
	if found {
		ciphertext, has, err := r.creds.Get(ctx, project.ID)
		if err != nil {
			return "", fmt.Errorf("read credential: %w", err)
		}
		if has {
			if r.sealer == nil {
				return "", ErrEncryptionKeyRequired
			}
			plaintext, err := r.sealer.Open(ciphertext)
			if err != nil {
				return "", fmt.Errorf("decrypt project token: %w", err)
			}
			return string(plaintext), nil
		}
	}
	return r.globalToken, nil
}
