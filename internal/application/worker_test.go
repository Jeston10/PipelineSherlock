package application

import (
	"context"
	"testing"
	"time"

	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

func TestWorker_DrainsThenStops(t *testing.T) {
	events := newFakeEventRepo()
	pipelines := &fakePipelineRepo{}
	mustInsert(t, events, "Pipeline Hook", pipelinePayloadJSON)
	// A second, distinct event.
	ev2, err := domain.NewWebhookEvent("proj_1", "Pipeline Hook", "d-2", []byte(pipelinePayloadJSON))
	if err != nil {
		t.Fatalf("new event: %v", err)
	}
	if _, err := events.Insert(context.Background(), ev2); err != nil {
		t.Fatalf("insert: %v", err)
	}

	svc := NewProcessEventsService(events, pipelines, nil, nil, nil)
	worker := NewWorker(svc, nil)
	worker.idleDelay = time.Millisecond // white-box: poll fast

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()

	// Wait until both events are processed.
	deadline := time.After(2 * time.Second)
	for {
		if events.statusAt(0) == domain.WebhookStatusProcessed &&
			events.statusAt(1) == domain.WebhookStatusProcessed {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("events not processed in time: %q, %q",
				events.statusAt(0), events.statusAt(1))
		case <-time.After(time.Millisecond):
		}
	}

	if n := pipelines.count(); n != 2 {
		t.Errorf("upserts = %d, want 2", n)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error on cancel: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after cancel")
	}
}
