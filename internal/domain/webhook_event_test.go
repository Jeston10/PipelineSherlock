package domain

import (
	"errors"
	"testing"
)

func TestNewWebhookEvent_Valid(t *testing.T) {
	ev, err := NewWebhookEvent("proj_1", "Pipeline Hook", "delivery-123", []byte(`{"ok":true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Status != WebhookStatusReceived {
		t.Errorf("status = %q, want received", ev.Status)
	}
	if ev.DeliveryKey != "delivery-123" {
		t.Errorf("delivery key = %q", ev.DeliveryKey)
	}
}

func TestNewWebhookEvent_Invariants(t *testing.T) {
	tests := []struct {
		name      string
		projectID ProjectID
		eventType string
		delivery  string
		payload   []byte
	}{
		{"empty project", "", "Pipeline Hook", "d1", []byte(`{}`)},
		{"empty event type", "proj_1", "", "d1", []byte(`{}`)},
		{"empty delivery key", "proj_1", "Pipeline Hook", "", []byte(`{}`)},
		{"empty payload", "proj_1", "Pipeline Hook", "d1", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewWebhookEvent(tc.projectID, tc.eventType, tc.delivery, tc.payload)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, ErrInvalidWebhookEvent) {
				t.Errorf("error %v does not wrap ErrInvalidWebhookEvent", err)
			}
		})
	}
}

func TestWebhookEventStatus_Valid(t *testing.T) {
	valid := []WebhookEventStatus{
		WebhookStatusReceived, WebhookStatusProcessing, WebhookStatusProcessed, WebhookStatusFailed,
	}
	for _, s := range valid {
		if !s.Valid() {
			t.Errorf("%q should be valid", s)
		}
	}
	if WebhookEventStatus("bogus").Valid() {
		t.Errorf("bogus status should be invalid")
	}
}
