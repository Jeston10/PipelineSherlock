package application

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
	"github.com/Jeston10/pipeline-sherlock/internal/platform/token"
)

// fakeProjectRepo is an in-memory ProjectRepository for tests.
type fakeProjectRepo struct {
	byGitLabID map[int64]domain.Project
	nextID     int
}

func newFakeProjectRepo() *fakeProjectRepo {
	return &fakeProjectRepo{byGitLabID: map[int64]domain.Project{}}
}

func (r *fakeProjectRepo) Create(_ context.Context, p domain.Project) (domain.Project, error) {
	if _, exists := r.byGitLabID[p.GitLabProjectID]; exists {
		return domain.Project{}, ErrProjectAlreadyExists
	}
	r.nextID++
	stored := p.WithPersistence(domain.ProjectID(strconv.Itoa(r.nextID)), time.Unix(0, 0).UTC())
	r.byGitLabID[p.GitLabProjectID] = stored
	return stored, nil
}

func (r *fakeProjectRepo) GetByGitLabID(_ context.Context, id int64) (domain.Project, bool, error) {
	p, ok := r.byGitLabID[id]
	return p, ok, nil
}

func (r *fakeProjectRepo) List(_ context.Context) ([]domain.Project, error) {
	out := make([]domain.Project, 0, len(r.byGitLabID))
	for _, p := range r.byGitLabID {
		out = append(out, p)
	}
	return out, nil
}

// fakeEventRepo is an in-memory idempotent WebhookEventRepository.
type fakeEventRepo struct {
	keys map[string]bool
}

func newFakeEventRepo() *fakeEventRepo {
	return &fakeEventRepo{keys: map[string]bool{}}
}

func (r *fakeEventRepo) Insert(_ context.Context, ev domain.WebhookEvent) (bool, error) {
	key := string(ev.ProjectID) + "|" + ev.DeliveryKey
	if r.keys[key] {
		return false, nil
	}
	r.keys[key] = true
	return true, nil
}

func TestRegisterProject_StoresHashNotPlaintext(t *testing.T) {
	repo := newFakeProjectRepo()
	svc := NewRegisterProjectService(repo)

	res, err := svc.Register(context.Background(), 1001, "checkout-service", "team/platform")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.WebhookSecret == "" {
		t.Fatal("expected a webhook secret to be returned")
	}
	if !res.Project.IsPersisted() {
		t.Error("returned project should be persisted")
	}
	if res.Project.WebhookSecretHash == res.WebhookSecret {
		t.Error("stored hash must not equal the plaintext secret")
	}
	if !token.Verify(res.Project.WebhookSecretHash, res.WebhookSecret) {
		t.Error("stored hash should verify against the returned secret")
	}
}

func TestIngestWebhook(t *testing.T) {
	ctx := context.Background()
	projects := newFakeProjectRepo()
	events := newFakeEventRepo()

	reg := NewRegisterProjectService(projects)
	res, err := reg.Register(ctx, 1001, "svc", "team")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	secret := res.WebhookSecret

	ingest := NewIngestWebhookService(projects, events)

	t.Run("unknown project", func(t *testing.T) {
		_, err := ingest.Ingest(ctx, IngestCommand{
			GitLabProjectID: 9999, Token: secret, EventType: "Pipeline Hook",
			DeliveryKey: "d1", Payload: []byte(`{}`),
		})
		if !errors.Is(err, ErrProjectNotFound) {
			t.Fatalf("err = %v, want ErrProjectNotFound", err)
		}
	})

	t.Run("invalid signature", func(t *testing.T) {
		_, err := ingest.Ingest(ctx, IngestCommand{
			GitLabProjectID: 1001, Token: "wrong-token", EventType: "Pipeline Hook",
			DeliveryKey: "d1", Payload: []byte(`{}`),
		})
		if !errors.Is(err, ErrInvalidSignature) {
			t.Fatalf("err = %v, want ErrInvalidSignature", err)
		}
	})

	t.Run("accepted then deduplicated", func(t *testing.T) {
		cmd := IngestCommand{
			GitLabProjectID: 1001, Token: secret, EventType: "Pipeline Hook",
			DeliveryKey: "delivery-abc", Payload: []byte(`{"object_kind":"pipeline"}`),
		}
		first, err := ingest.Ingest(ctx, cmd)
		if err != nil {
			t.Fatalf("first ingest: %v", err)
		}
		if !first.Accepted {
			t.Error("first delivery should be Accepted")
		}
		second, err := ingest.Ingest(ctx, cmd)
		if err != nil {
			t.Fatalf("second ingest: %v", err)
		}
		if second.Accepted {
			t.Error("redelivered event should be de-duplicated (not Accepted)")
		}
	})
}
