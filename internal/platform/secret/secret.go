// Package secret resolves sensitive configuration values (connection strings,
// tokens) through a provider abstraction, so the source can change — environment
// variables for local development, mounted files in production, a cloud secret
// manager later — without touching the config loader.
package secret

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Provider resolves a named secret to its value. A missing optional secret
// returns an empty string with a nil error; the caller (config) decides whether
// a given secret is required.
type Provider interface {
	Get(ctx context.Context, name string) (string, error)
}

// Selection errors from FromEnv.
var (
	// ErrSecretsDirRequired is returned when the file provider is selected but no
	// SECRETS_DIR is configured.
	ErrSecretsDirRequired = errors.New("SECRETS_DIR is required when SECRET_PROVIDER=file")
	// ErrUnknownProvider is returned for an unrecognised SECRET_PROVIDER value.
	ErrUnknownProvider = errors.New("unknown SECRET_PROVIDER")
)

// EnvProvider reads secrets from environment variables: the secret name is the
// variable name. It is the default for local development.
type EnvProvider struct {
	getenv func(string) string
}

// NewEnvProvider builds an environment-backed provider. A nil getenv falls back
// to the real process environment.
func NewEnvProvider(getenv func(string) string) *EnvProvider {
	if getenv == nil {
		getenv = os.Getenv
	}
	return &EnvProvider{getenv: getenv}
}

// Get returns the environment variable named by the secret (empty when unset).
func (p *EnvProvider) Get(_ context.Context, name string) (string, error) {
	return p.getenv(name), nil
}

// FileProvider reads each secret from a file named after the secret inside a
// directory — the convention used by Docker and Kubernetes mounted secrets
// (e.g. /run/secrets/DATABASE_URL) — keeping secret values out of the process
// environment. A missing file is treated as an unset (empty) secret.
type FileProvider struct {
	dir string
}

// NewFileProvider builds a file-backed provider rooted at dir.
func NewFileProvider(dir string) *FileProvider {
	return &FileProvider{dir: dir}
}

// Get reads the secret from <dir>/<name>, trimming surrounding whitespace. The
// name must be a bare file name: a name containing a path separator or ".." is
// rejected so a secret name can never escape the directory.
func (p *FileProvider) Get(_ context.Context, name string) (string, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid secret name %q", name)
	}
	data, err := os.ReadFile(filepath.Join(p.dir, name))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read secret %q: %w", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// FromEnv selects a secret provider from the environment. SECRET_PROVIDER picks
// the backend: "env" (the default) reads from environment variables; "file"
// reads from SECRETS_DIR. A nil getenv falls back to the real environment.
func FromEnv(getenv func(string) string) (Provider, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	switch strings.ToLower(strings.TrimSpace(getenv("SECRET_PROVIDER"))) {
	case "", "env":
		return NewEnvProvider(getenv), nil
	case "file":
		dir := strings.TrimSpace(getenv("SECRETS_DIR"))
		if dir == "" {
			return nil, ErrSecretsDirRequired
		}
		return NewFileProvider(dir), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownProvider, strings.TrimSpace(getenv("SECRET_PROVIDER")))
	}
}
