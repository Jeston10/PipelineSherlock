package secret

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// compile-time assertions that both adapters satisfy the port.
var (
	_ Provider = (*EnvProvider)(nil)
	_ Provider = (*FileProvider)(nil)
)

func TestEnvProvider(t *testing.T) {
	p := NewEnvProvider(func(k string) string {
		if k == "DATABASE_URL" {
			return "postgres://localhost/ps"
		}
		return ""
	})

	got, err := p.Get(context.Background(), "DATABASE_URL")
	if err != nil || got != "postgres://localhost/ps" {
		t.Fatalf("Get(DATABASE_URL) = (%q, %v), want the value", got, err)
	}
	if got, err := p.Get(context.Background(), "MISSING"); err != nil || got != "" {
		t.Fatalf("Get(MISSING) = (%q, %v), want (\"\", nil)", got, err)
	}
}

func TestFileProvider(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "DATABASE_URL"),
		[]byte("postgres://localhost/ps\n"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	p := NewFileProvider(dir)

	t.Run("trims file contents", func(t *testing.T) {
		got, err := p.Get(context.Background(), "DATABASE_URL")
		if err != nil || got != "postgres://localhost/ps" {
			t.Fatalf("Get = (%q, %v), want the trimmed value", got, err)
		}
	})

	t.Run("missing file is empty", func(t *testing.T) {
		got, err := p.Get(context.Background(), "GITLAB_TOKEN")
		if err != nil || got != "" {
			t.Fatalf("Get(missing) = (%q, %v), want (\"\", nil)", got, err)
		}
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		if _, err := p.Get(context.Background(), "../etc/passwd"); err == nil {
			t.Fatal("expected an error for a name that escapes the directory")
		}
	})
}

func TestFromEnv(t *testing.T) {
	t.Run("defaults to env", func(t *testing.T) {
		p, err := FromEnv(func(string) string { return "" })
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := p.(*EnvProvider); !ok {
			t.Errorf("provider = %T, want *EnvProvider", p)
		}
	})

	t.Run("file requires a directory", func(t *testing.T) {
		env := map[string]string{"SECRET_PROVIDER": "file"}
		if _, err := FromEnv(func(k string) string { return env[k] }); !errors.Is(err, ErrSecretsDirRequired) {
			t.Fatalf("err = %v, want ErrSecretsDirRequired", err)
		}
	})

	t.Run("file with a directory", func(t *testing.T) {
		env := map[string]string{"SECRET_PROVIDER": "file", "SECRETS_DIR": "/run/secrets"}
		p, err := FromEnv(func(k string) string { return env[k] })
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := p.(*FileProvider); !ok {
			t.Errorf("provider = %T, want *FileProvider", p)
		}
	})

	t.Run("unknown provider errors", func(t *testing.T) {
		env := map[string]string{"SECRET_PROVIDER": "wat"}
		if _, err := FromEnv(func(k string) string { return env[k] }); !errors.Is(err, ErrUnknownProvider) {
			t.Fatalf("err = %v, want ErrUnknownProvider", err)
		}
	})
}
