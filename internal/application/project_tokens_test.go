package application

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
	"github.com/Jeston10/pipeline-sherlock/internal/platform/crypto"
)

// fakeCredentialRepo is an in-memory ProjectCredentialRepository.
type fakeCredentialRepo struct {
	byProject map[domain.ProjectID][]byte
	getErr    error
	upsertErr error
}

func newFakeCredentialRepo() *fakeCredentialRepo {
	return &fakeCredentialRepo{byProject: map[domain.ProjectID][]byte{}}
}

func (r *fakeCredentialRepo) Upsert(_ context.Context, projectID domain.ProjectID, ciphertext []byte) error {
	if r.upsertErr != nil {
		return r.upsertErr
	}
	cp := make([]byte, len(ciphertext))
	copy(cp, ciphertext)
	r.byProject[projectID] = cp
	return nil
}

func (r *fakeCredentialRepo) Get(_ context.Context, projectID domain.ProjectID) ([]byte, bool, error) {
	if r.getErr != nil {
		return nil, false, r.getErr
	}
	ct, ok := r.byProject[projectID]
	return ct, ok, nil
}

func testSealer(t *testing.T) *crypto.Sealer {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	s, err := crypto.NewSealer(key)
	if err != nil {
		t.Fatalf("sealer: %v", err)
	}
	return s
}

// registerOwned registers a project under an owner and returns it.
func registerOwned(t *testing.T, projects *fakeProjectRepo, owner string, gitlabID int64) domain.Project {
	t.Helper()
	res, err := NewRegisterProjectService(projects).Register(context.Background(), owner, gitlabID, "svc", "team/platform")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return res.Project
}

func TestSetProjectToken_StoresCiphertextOnly(t *testing.T) {
	ctx := context.Background()
	projects := newFakeProjectRepo()
	creds := newFakeCredentialRepo()
	sealer := testSealer(t)
	registerOwned(t, projects, "acme", 1001)

	svc := NewSetProjectTokenService(projects, creds, sealer)
	if err := svc.Set(ctx, "acme", 1001, "glpat-secret"); err != nil {
		t.Fatalf("set: %v", err)
	}

	var stored []byte
	for _, ct := range creds.byProject {
		stored = ct
	}
	if stored == nil {
		t.Fatal("no credential stored")
	}
	if bytes.Contains(stored, []byte("glpat-secret")) {
		t.Fatal("stored credential contains the plaintext token")
	}
	got, err := sealer.Open(stored)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if string(got) != "glpat-secret" {
		t.Errorf("decrypted = %q, want glpat-secret", got)
	}
}

func TestSetProjectToken_CrossOwnerHidden(t *testing.T) {
	ctx := context.Background()
	projects := newFakeProjectRepo()
	creds := newFakeCredentialRepo()
	registerOwned(t, projects, "acme", 1001)

	svc := NewSetProjectTokenService(projects, creds, testSealer(t))
	if err := svc.Set(ctx, "other", 1001, "glpat-x"); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("err = %v, want ErrProjectNotFound", err)
	}
	if len(creds.byProject) != 0 {
		t.Error("no credential should be stored for a cross-owner set")
	}
}

func TestSetProjectToken_RequiresKeyAndToken(t *testing.T) {
	ctx := context.Background()
	projects := newFakeProjectRepo()
	creds := newFakeCredentialRepo()
	registerOwned(t, projects, "acme", 1001)

	noKey := NewSetProjectTokenService(projects, creds, nil)
	if err := noKey.Set(ctx, "acme", 1001, "glpat-x"); !errors.Is(err, ErrEncryptionKeyRequired) {
		t.Fatalf("err = %v, want ErrEncryptionKeyRequired", err)
	}
	withKey := NewSetProjectTokenService(projects, creds, testSealer(t))
	if err := withKey.Set(ctx, "acme", 1001, "  "); !errors.Is(err, ErrEmptyToken) {
		t.Fatalf("err = %v, want ErrEmptyToken", err)
	}
}

func TestProjectTokenResolver_PerProjectBeatsGlobal(t *testing.T) {
	ctx := context.Background()
	projects := newFakeProjectRepo()
	creds := newFakeCredentialRepo()
	sealer := testSealer(t)
	registerOwned(t, projects, "acme", 1001)
	if err := NewSetProjectTokenService(projects, creds, sealer).Set(ctx, "acme", 1001, "glpat-project"); err != nil {
		t.Fatalf("set: %v", err)
	}

	resolver := NewProjectTokenResolver(projects, creds, sealer, "glpat-global")
	tok, err := resolver.TokenForGitLabProject(ctx, 1001)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tok != "glpat-project" {
		t.Errorf("token = %q, want the project token", tok)
	}
}

func TestProjectTokenResolver_FallsBackToGlobal(t *testing.T) {
	ctx := context.Background()
	projects := newFakeProjectRepo()
	creds := newFakeCredentialRepo()
	registerOwned(t, projects, "acme", 1001)

	resolver := NewProjectTokenResolver(projects, creds, testSealer(t), "glpat-global")
	tok, err := resolver.TokenForGitLabProject(ctx, 1001)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tok != "glpat-global" {
		t.Errorf("token = %q, want the global token", tok)
	}
}

func TestProjectTokenResolver_EmptyWhenNoTokenAnywhere(t *testing.T) {
	ctx := context.Background()
	projects := newFakeProjectRepo()
	creds := newFakeCredentialRepo()
	registerOwned(t, projects, "acme", 1001)

	resolver := NewProjectTokenResolver(projects, creds, testSealer(t), "")
	tok, err := resolver.TokenForGitLabProject(ctx, 1001)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tok != "" {
		t.Errorf("token = %q, want empty (adapter maps to ErrNoGitLabToken)", tok)
	}
}

func TestProjectTokenResolver_UnknownProjectFallsBackToGlobal(t *testing.T) {
	ctx := context.Background()
	resolver := NewProjectTokenResolver(newFakeProjectRepo(), newFakeCredentialRepo(), testSealer(t), "glpat-global")
	tok, err := resolver.TokenForGitLabProject(ctx, 9999)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tok != "glpat-global" {
		t.Errorf("token = %q, want global for an unknown project", tok)
	}
}
