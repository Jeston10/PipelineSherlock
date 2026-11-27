package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// envMap builds a deterministic Getenv backed by a map, so tests never touch
// the real process environment.
func envMap(m map[string]string) Getenv {
	return func(key string) string { return m[key] }
}

func TestLoad_MinimalValidConfig(t *testing.T) {
	cfg, err := Load(envMap(map[string]string{
		"DATABASE_URL": "postgres://user:pass@localhost:5432/ps?sslmode=disable",
	}))
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
	if cfg.Env != "development" {
		t.Errorf("Env default = %q, want development", cfg.Env)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr default = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.ShutdownTimeout != 15*time.Second {
		t.Errorf("ShutdownTimeout default = %s, want 15s", cfg.ShutdownTimeout)
	}
	if cfg.RedisEnabled() {
		t.Errorf("RedisEnabled() = true, want false when REDIS_URL unset")
	}
	if cfg.GitLabBaseURL != "https://gitlab.com" {
		t.Errorf("GitLabBaseURL default = %q, want https://gitlab.com", cfg.GitLabBaseURL)
	}
	if cfg.GitLabConfigured() {
		t.Errorf("GitLabConfigured() = true, want false when GITLAB_TOKEN unset")
	}
}

func TestLoad_GitLabConfigured(t *testing.T) {
	cfg, err := Load(envMap(map[string]string{
		"DATABASE_URL":    "postgres://localhost/ps",
		"GITLAB_BASE_URL": "https://gitlab.example.com",
		"GITLAB_TOKEN":    "glpat-secret",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GitLabBaseURL != "https://gitlab.example.com" {
		t.Errorf("GitLabBaseURL = %q, want the override", cfg.GitLabBaseURL)
	}
	if !cfg.GitLabConfigured() {
		t.Errorf("GitLabConfigured() = false, want true when GITLAB_TOKEN set")
	}
}

func TestLoad_WriteBack(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		wantFlag  bool
		wantActiv bool
	}{
		{
			name:      "disabled by default",
			env:       map[string]string{"DATABASE_URL": "postgres://localhost/ps"},
			wantFlag:  false,
			wantActiv: false,
		},
		{
			name:      "flag set but no token is not active",
			env:       map[string]string{"DATABASE_URL": "postgres://localhost/ps", "GITLAB_WRITEBACK_ENABLED": "true"},
			wantFlag:  true,
			wantActiv: false,
		},
		{
			name:      "flag set with token is active",
			env:       map[string]string{"DATABASE_URL": "postgres://localhost/ps", "GITLAB_TOKEN": "glpat-x", "GITLAB_WRITEBACK_ENABLED": "true"},
			wantFlag:  true,
			wantActiv: true,
		},
		{
			name:      "unparseable flag is off",
			env:       map[string]string{"DATABASE_URL": "postgres://localhost/ps", "GITLAB_TOKEN": "glpat-x", "GITLAB_WRITEBACK_ENABLED": "yep"},
			wantFlag:  false,
			wantActiv: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(envMap(tt.env))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.WriteBackEnabled != tt.wantFlag {
				t.Errorf("WriteBackEnabled = %v, want %v", cfg.WriteBackEnabled, tt.wantFlag)
			}
			if cfg.WriteBackActive() != tt.wantActiv {
				t.Errorf("WriteBackActive() = %v, want %v", cfg.WriteBackActive(), tt.wantActiv)
			}
		})
	}
}

func TestLoad_Validation(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string // substring expected in the error, empty means no error
	}{
		{
			name:    "missing database url",
			env:     map[string]string{},
			wantErr: "DATABASE_URL is required",
		},
		{
			name: "invalid duration",
			env: map[string]string{
				"DATABASE_URL":     "postgres://localhost/ps",
				"SHUTDOWN_TIMEOUT": "not-a-duration",
			},
			wantErr: "not a valid duration",
		},
		{
			name: "valid with overrides",
			env: map[string]string{
				"DATABASE_URL":     "postgres://localhost/ps",
				"APP_ENV":          "production",
				"HTTP_ADDR":        "127.0.0.1:9000",
				"REDIS_URL":        "redis://localhost:6379/0",
				"SHUTDOWN_TIMEOUT": "30s",
			},
			wantErr: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Load(envMap(tc.env))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil (cfg=%+v)", tc.wantErr, cfg)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestLoad_SecondsInteger(t *testing.T) {
	cfg, err := Load(envMap(map[string]string{
		"DATABASE_URL":     "postgres://localhost/ps",
		"SHUTDOWN_TIMEOUT": "20",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ShutdownTimeout != 20*time.Second {
		t.Errorf("ShutdownTimeout = %s, want 20s", cfg.ShutdownTimeout)
	}
}

// TestLoad_FileSecretProvider resolves DATABASE_URL from a mounted-secret file
// rather than the environment, exercising the SECRET_PROVIDER=file path.
func TestLoad_FileSecretProvider(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "DATABASE_URL"),
		[]byte("postgres://localhost/ps\n"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	cfg, err := Load(envMap(map[string]string{
		"SECRET_PROVIDER": "file",
		"SECRETS_DIR":     dir,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "postgres://localhost/ps" {
		t.Errorf("DatabaseURL = %q, want the file's trimmed value", cfg.DatabaseURL)
	}
}

// TestLoad_UnknownSecretProvider fails fast when SECRET_PROVIDER is unrecognised.
func TestLoad_UnknownSecretProvider(t *testing.T) {
	_, err := Load(envMap(map[string]string{
		"DATABASE_URL":    "postgres://localhost/ps",
		"SECRET_PROVIDER": "wat",
	}))
	if err == nil {
		t.Fatal("expected an error for an unknown SECRET_PROVIDER")
	}
}

// TestRedacted guards against the single most important logging mistake:
// leaking database/redis credentials into logs.
func TestRedacted_NeverLeaksCredentials(t *testing.T) {
	cfg, err := Load(envMap(map[string]string{
		"DATABASE_URL": "postgres://admin:supersecret@db.internal:5432/ps",
		"REDIS_URL":    "redis://:topsecret@cache.internal:6379/0",
		"GITLAB_TOKEN": "glpat-tokensecret",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := cfg.Redacted()
	for _, leak := range []string{"supersecret", "topsecret", "admin", "glpat-tokensecret"} {
		if strings.Contains(out, leak) {
			t.Errorf("Redacted() leaked %q: %s", leak, out)
		}
	}
	// It should still carry useful, non-sensitive shape.
	if !strings.Contains(out, "db.internal") {
		t.Errorf("Redacted() dropped host context: %s", out)
	}
}
