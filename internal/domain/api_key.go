package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// maxAPIKeyNameLen bounds the human-readable label stored with a key.
const maxAPIKeyNameLen = 100

var (
	// ErrInvalidAPIKey is the umbrella error for an API key that fails its
	// identity invariants.
	ErrInvalidAPIKey = errors.New("invalid api key")

	// ErrEmptyAPIKeyName is returned when the key's label is blank.
	ErrEmptyAPIKeyName = fmt.Errorf("%w: name must not be empty", ErrInvalidAPIKey)
	// ErrAPIKeyNameTooLong is returned when the label exceeds the maximum length.
	ErrAPIKeyNameTooLong = fmt.Errorf("%w: name exceeds maximum length", ErrInvalidAPIKey)
	// ErrEmptyAPIKeyHash is returned when no key hash is supplied.
	ErrEmptyAPIKeyHash = fmt.Errorf("%w: key hash required", ErrInvalidAPIKey)
)

// APIKey is a credential that authenticates callers of the /api/v1 surface.
// Only the SHA-256 hash of the key is ever persisted; the plaintext is shown to
// the operator once at creation and never stored or logged. Immutable value.
type APIKey struct {
	ID         string
	Name       string
	KeyHash    string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// NewAPIKey builds a key record from a label and the hash of its secret,
// validating the identity needed to persist and later match it.
func NewAPIKey(name, keyHash string) (APIKey, error) {
	trimmed := strings.TrimSpace(name)

	var problems []error
	if trimmed == "" {
		problems = append(problems, ErrEmptyAPIKeyName)
	}
	if len(trimmed) > maxAPIKeyNameLen {
		problems = append(problems, ErrAPIKeyNameTooLong)
	}
	if keyHash == "" {
		problems = append(problems, ErrEmptyAPIKeyHash)
	}
	if len(problems) > 0 {
		return APIKey{}, errors.Join(problems...)
	}
	return APIKey{Name: trimmed, KeyHash: keyHash}, nil
}
