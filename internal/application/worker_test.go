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

	svc := NewProcessEventsService(events, pipelines, nil, nil)
	worker := NewWorker(svc, nil)
	worker.idleDelay = time.Millisecond // white-box: poll fast

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()

	// Wait until both events are processed.
	deadline := time.After(2 * time.Second)
	for {
		if events.events[0].Status == domain.WebhookStatusProcessed &&
			events.events[1].Status == domain.WebhookStatusProcessed {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("events not processed in time: %q, %q",
				events.events[0].Status, events.events[1].Status)
		case <-time.After(time.Millisecond):
		}
	}

	if len(pipelines.upserts) != 2 {
		t.Errorf("upserts = %d, want 2", len(pipelines.upserts))
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
