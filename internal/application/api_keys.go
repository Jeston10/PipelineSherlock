package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
	"github.com/Jeston10/pipeline-sherlock/internal/platform/token"
)

// ErrUnauthenticated means the presented API key was absent, malformed or did
// not match any stored key. It is deliberately opaque: callers must not be able
// to distinguish "unknown key" from "no key" from the response.
var ErrUnauthenticated = errors.New("unauthenticated")

// APIKeyRepository persists API keys and looks them up by hash. Only the hash
// is stored, so lookup is an equality match on the hash (never the secret).
// Defined here, consumer-side; the Postgres adapter implements it.
type APIKeyRepository interface {
	// Create stores a new key and returns it with its assigned ID and CreatedAt.
	Create(ctx context.Context, k domain.APIKey) (domain.APIKey, error)
	// FindByHash returns the key whose stored hash equals keyHash. found is false
	// (nil error) when no key matches.
	FindByHash(ctx context.Context, keyHash string) (key domain.APIKey, found bool, err error)
}

// APIKeyService issues new API keys and authenticates presented ones.
type APIKeyService struct {
	repo APIKeyRepository
}

// NewAPIKeyService constructs the service over the given repository.
func NewAPIKeyService(repo APIKeyRepository) *APIKeyService {
	return &APIKeyService{repo: repo}
}

// IssueAPIKeyResult carries a newly issued key. PlaintextKey is shown to the
// operator exactly once and is never stored or logged.
type IssueAPIKeyResult struct {
	APIKey       domain.APIKey
	PlaintextKey string
}

// Issue generates a new high-entropy key, stores only its SHA-256 hash under the
// given name, and returns the plaintext once for the operator to record.
func (s *APIKeyService) Issue(ctx context.Context, name string) (IssueAPIKeyResult, error) {
	plaintext, err := token.Generate()
	if err != nil {
		return IssueAPIKeyResult{}, fmt.Errorf("generate api key: %w", err)
	}
	key, err := domain.NewAPIKey(name, token.Hash(plaintext))
	if err != nil {
		return IssueAPIKeyResult{}, err
	}
	stored, err := s.repo.Create(ctx, key)
	if err != nil {
		return IssueAPIKeyResult{}, fmt.Errorf("store api key: %w", err)
	}
	return IssueAPIKeyResult{APIKey: stored, PlaintextKey: plaintext}, nil
}

// Authenticate verifies a presented plaintext key by hashing it and matching the
// hash against stored keys. It returns the matching key on success, or
// ErrUnauthenticated when the key is empty or unknown. Because the comparison is
// over SHA-256 hashes (never the secret), a direct indexed lookup leaks nothing
// useful about any key's plaintext.
func (s *APIKeyService) Authenticate(ctx context.Context, plaintextKey string) (domain.APIKey, error) {
	if strings.TrimSpace(plaintextKey) == "" {
		return domain.APIKey{}, ErrUnauthenticated
	}
	key, found, err := s.repo.FindByHash(ctx, token.Hash(plaintextKey))
	if err != nil {
		return domain.APIKey{}, fmt.Errorf("lookup api key: %w", err)
	}
	if !found {
		return domain.APIKey{}, ErrUnauthenticated
	}
	return key, nil
}
