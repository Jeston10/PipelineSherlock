package application

import (
	"context"
	"errors"
	"testing"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
	"github.com/Jeston10/pipeline-sherlock/internal/platform/token"
)

// fakeAPIKeyRepo is an in-memory APIKeyRepository keyed by hash.
type fakeAPIKeyRepo struct {
	byHash     map[string]domain.APIKey
	createErr  error
	lookupErr  error
	lastStored domain.APIKey
}

func newFakeAPIKeyRepo() *fakeAPIKeyRepo {
	return &fakeAPIKeyRepo{byHash: map[string]domain.APIKey{}}
}

func (f *fakeAPIKeyRepo) Create(_ context.Context, k domain.APIKey) (domain.APIKey, error) {
	if f.createErr != nil {
		return domain.APIKey{}, f.createErr
	}
	k.ID = "key_" + k.Name
	f.byHash[k.KeyHash] = k
	f.lastStored = k
	return k, nil
}

func (f *fakeAPIKeyRepo) FindByHash(_ context.Context, keyHash string) (domain.APIKey, bool, error) {
	if f.lookupErr != nil {
		return domain.APIKey{}, false, f.lookupErr
	}
	k, ok := f.byHash[keyHash]
	return k, ok, nil
}

func TestAPIKeyService_Issue_StoresHashNotPlaintext(t *testing.T) {
	repo := newFakeAPIKeyRepo()
	svc := NewAPIKeyService(repo)

	res, err := svc.Issue(context.Background(), "ci-dashboard", "acme")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if res.PlaintextKey == "" {
		t.Fatal("plaintext key must be returned once")
	}
	if repo.lastStored.OwnerID != "acme" {
		t.Errorf("stored owner = %q, want acme", repo.lastStored.OwnerID)
	}
	if repo.lastStored.KeyHash == res.PlaintextKey {
		t.Fatal("stored value must be a hash, never the plaintext")
	}
	if repo.lastStored.KeyHash != token.Hash(res.PlaintextKey) {
		t.Errorf("stored hash = %q, want hash of the plaintext", repo.lastStored.KeyHash)
	}
	if res.APIKey.ID == "" {
		t.Error("issued key should carry its assigned id")
	}
}

func TestAPIKeyService_Issue_InvalidName(t *testing.T) {
	svc := NewAPIKeyService(newFakeAPIKeyRepo())
	if _, err := svc.Issue(context.Background(), "   ", "acme"); !errors.Is(err, domain.ErrInvalidAPIKey) {
		t.Fatalf("err = %v, want ErrInvalidAPIKey", err)
	}
}

func TestAPIKeyService_Issue_MissingOwner(t *testing.T) {
	svc := NewAPIKeyService(newFakeAPIKeyRepo())
	if _, err := svc.Issue(context.Background(), "ci-dashboard", "  "); !errors.Is(err, domain.ErrInvalidAPIKey) {
		t.Fatalf("err = %v, want ErrInvalidAPIKey", err)
	}
}

func TestAPIKeyService_Authenticate(t *testing.T) {
	repo := newFakeAPIKeyRepo()
	svc := NewAPIKeyService(repo)
	issued, err := svc.Issue(context.Background(), "ci-dashboard", "acme")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	t.Run("valid key", func(t *testing.T) {
		key, err := svc.Authenticate(context.Background(), issued.PlaintextKey)
		if err != nil {
			t.Fatalf("authenticate: %v", err)
		}
		if key.ID != issued.APIKey.ID {
			t.Errorf("key id = %q, want %q", key.ID, issued.APIKey.ID)
		}
		if key.OwnerID != "acme" {
			t.Errorf("key owner = %q, want acme", key.OwnerID)
		}
	})

	t.Run("empty key", func(t *testing.T) {
		if _, err := svc.Authenticate(context.Background(), ""); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrUnauthenticated", err)
		}
	})

	t.Run("unknown key", func(t *testing.T) {
		if _, err := svc.Authenticate(context.Background(), "not-a-real-key"); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrUnauthenticated", err)
		}
	})
}
