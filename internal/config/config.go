// Package config loads and validates service configuration from the
// environment. Configuration is validated once at startup and the process
// fails fast with an explicit error rather than starting in a half-configured
// state.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for the PipelineSherlock server.
// Values are immutable once loaded.
type Config struct {
	// Env is the deployment environment name (e.g. "development", "production").
	Env string
	// HTTPAddr is the listen address for the public API, e.g. ":8080".
	HTTPAddr string
	// DatabaseURL is the PostgreSQL connection string (source of truth).
	DatabaseURL string
	// RedisURL is optional; empty means Redis-backed features are disabled.
	RedisURL string
	// ShutdownTimeout bounds graceful shutdown.
	ShutdownTimeout time.Duration
	// ReadTimeout / WriteTimeout bound inbound HTTP handling.
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	// GitLabBaseURL is the GitLab API base (e.g. https://gitlab.com). The
	// background worker reconciles pipeline/job state against this host.
	GitLabBaseURL string
	// GitLabToken is an optional personal/project access token. When empty the
	// worker is disabled and the service only ingests webhooks.
	GitLabToken string
	// WriteBackEnabled opts in to posting incident diagnoses back to the merge
	// request a failing pipeline belongs to. It takes effect only alongside a
	// configured GitLab token (see WriteBackActive).
	WriteBackEnabled bool
}

// Default values applied when an optional variable is unset.
const (
	defaultHTTPAddr        = ":8080"
	defaultShutdownTimeout = 15 * time.Second
	defaultReadTimeout     = 15 * time.Second
	defaultWriteTimeout    = 30 * time.Second
	defaultGitLabBaseURL   = "https://gitlab.com"
)

// Getenv is the function used to read environment variables. It is a variable
// so tests can inject a deterministic source without touching the real
// process environment.
type Getenv func(key string) string

// Load reads configuration using the given lookup function and validates it.
// Passing os.Getenv reads the real environment. A non-nil error means the
// configuration is unusable and the caller must not continue.
func Load(getenv Getenv) (Config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}

	cfg := Config{
		Env:              firstNonEmpty(getenv("APP_ENV"), "development"),
		HTTPAddr:         firstNonEmpty(getenv("HTTP_ADDR"), defaultHTTPAddr),
		DatabaseURL:      strings.TrimSpace(getenv("DATABASE_URL")),
		RedisURL:         strings.TrimSpace(getenv("REDIS_URL")),
		ShutdownTimeout:  defaultShutdownTimeout,
		ReadTimeout:      defaultReadTimeout,
		WriteTimeout:     defaultWriteTimeout,
		GitLabBaseURL:    firstNonEmpty(getenv("GITLAB_BASE_URL"), defaultGitLabBaseURL),
		GitLabToken:      strings.TrimSpace(getenv("GITLAB_TOKEN")),
		WriteBackEnabled: boolEnv(getenv, "GITLAB_WRITEBACK_ENABLED"),
	}

	if d, err := durationEnv(getenv, "SHUTDOWN_TIMEOUT"); err != nil {
		return Config{}, err
	} else if d > 0 {
		cfg.ShutdownTimeout = d
	}
	if d, err := durationEnv(getenv, "HTTP_READ_TIMEOUT"); err != nil {
		return Config{}, err
	} else if d > 0 {
		cfg.ReadTimeout = d
	}
	if d, err := durationEnv(getenv, "HTTP_WRITE_TIMEOUT"); err != nil {
		return Config{}, err
	} else if d > 0 {
		cfg.WriteTimeout = d
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// validate enforces required fields and sane values. It aggregates problems so
// one run reports every misconfiguration rather than failing one at a time.
func (c Config) validate() error {
	var problems []string

	if c.DatabaseURL == "" {
		problems = append(problems, "DATABASE_URL is required")
	}
	if !strings.HasPrefix(c.HTTPAddr, ":") && !strings.Contains(c.HTTPAddr, ":") {
		problems = append(problems, fmt.Sprintf("HTTP_ADDR %q is not a valid listen address", c.HTTPAddr))
	}
	if c.ShutdownTimeout <= 0 {
		problems = append(problems, "SHUTDOWN_TIMEOUT must be positive")
	}
	if c.GitLabBaseURL == "" || !strings.Contains(c.GitLabBaseURL, "://") {
		problems = append(problems, fmt.Sprintf("GITLAB_BASE_URL %q is not a valid URL", c.GitLabBaseURL))
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid configuration: %s", strings.Join(problems, "; "))
	}
	return nil
}

// Redacted returns a copy-safe string representation that never exposes
// credentials. It is intended for startup logging.
func (c Config) Redacted() string {
	return fmt.Sprintf(
		"env=%s http_addr=%s database=%s redis=%s gitlab=%s gitlab_token=%s writeback=%t shutdown_timeout=%s",
		c.Env, c.HTTPAddr, redactURL(c.DatabaseURL), redactURL(c.RedisURL),
		c.GitLabBaseURL, redactSecret(c.GitLabToken), c.WriteBackActive(), c.ShutdownTimeout,
	)
}

// RedisEnabled reports whether a Redis URL was configured.
func (c Config) RedisEnabled() bool { return c.RedisURL != "" }

// GitLabConfigured reports whether a GitLab token is present, which enables the
// background reconciliation worker.
func (c Config) GitLabConfigured() bool { return c.GitLabToken != "" }

// WriteBackActive reports whether merge-request write-back should run: it needs
// both the opt-in flag and a configured GitLab token (posting requires the API).
func (c Config) WriteBackActive() bool { return c.WriteBackEnabled && c.GitLabConfigured() }

// boolEnv reads a boolean environment variable. An unset or unparseable value is
// false, so an opt-in flag stays off unless explicitly enabled.
func boolEnv(getenv Getenv, key string) bool {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return false
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return v
}

func durationEnv(getenv Getenv, key string) (time.Duration, error) {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return 0, nil
	}
	// Allow either a Go duration ("15s") or a plain integer number of seconds.
	if d, err := time.ParseDuration(raw); err == nil {
		return d, nil
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return time.Duration(n) * time.Second, nil
	}
	return 0, fmt.Errorf("%s %q is not a valid duration", key, raw)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// redactURL hides any userinfo (credentials) embedded in a connection string
// while keeping enough shape to be useful in logs. It never returns the raw
// value for a non-empty URL.
func redactURL(raw string) string {
	if raw == "" {
		return "(unset)"
	}
	at := strings.LastIndex(raw, "@")
	scheme := strings.Index(raw, "://")
	if at > 0 && scheme >= 0 && at > scheme {
		return raw[:scheme+3] + "***@" + raw[at+1:]
	}
	return "***"
}

// redactSecret reports presence without revealing the value.
func redactSecret(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "(unset)"
	}
	return "***"
}

// ErrMissingDatabaseURL is kept for callers that want to detect the common case
// explicitly; Load returns an aggregated error, so this is informational.
var ErrMissingDatabaseURL = errors.New("DATABASE_URL is required")
