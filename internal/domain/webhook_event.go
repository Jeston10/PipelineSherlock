package domain

import (
	"errors"
	"fmt"
	"time"
)

// WebhookEventStatus is the processing state of an inbox event.
type WebhookEventStatus string

const (
	// WebhookStatusReceived is the initial state: persisted, not yet processed.
	WebhookStatusReceived WebhookEventStatus = "received"
	// WebhookStatusProcessing marks an event a worker has claimed.
	WebhookStatusProcessing WebhookEventStatus = "processing"
	// WebhookStatusProcessed marks an event whose processing succeeded.
	WebhookStatusProcessed WebhookEventStatus = "processed"
	// WebhookStatusFailed marks an event whose processing failed terminally.
	WebhookStatusFailed WebhookEventStatus = "failed"
)

// Valid reports whether s is a known status.
func (s WebhookEventStatus) Valid() bool {
	switch s {
	case WebhookStatusReceived, WebhookStatusProcessing, WebhookStatusProcessed, WebhookStatusFailed:
		return true
	default:
		return false
	}
}

// WebhookEvent validation sentinels.
var (
	ErrInvalidWebhookEvent = errors.New("invalid webhook event")

	ErrEmptyEventType   = fmt.Errorf("%w: event type must not be empty", ErrInvalidWebhookEvent)
	ErrEmptyDeliveryKey = fmt.Errorf("%w: delivery key must not be empty", ErrInvalidWebhookEvent)
	ErrEmptyPayload     = fmt.Errorf("%w: payload must not be empty", ErrInvalidWebhookEvent)
)

// WebhookEvent is one GitLab webhook delivery captured in the durable inbox.
// The (ProjectID, DeliveryKey) pair is unique, which is how redelivered events
// are de-duplicated.
type WebhookEvent struct {
	ID          string
	ProjectID   ProjectID
	EventType   string
	DeliveryKey string
	Payload     []byte
	Status      WebhookEventStatus
	ReceivedAt  time.Time
	ProcessedAt *time.Time
	ErrorCode   string
}

// NewWebhookEvent validates and constructs an inbox event in the "received"
// state. ID and ReceivedAt are assigned by the persistence layer.
func NewWebhookEvent(projectID ProjectID, eventType, deliveryKey string, payload []byte) (WebhookEvent, error) {
	var problems []error
	if projectID == "" {
		problems = append(problems, fmt.Errorf("%w: project id must not be empty", ErrInvalidWebhookEvent))
	}
	if eventType == "" {
		problems = append(problems, ErrEmptyEventType)
	}
	if deliveryKey == "" {
		problems = append(problems, ErrEmptyDeliveryKey)
	}
	if len(payload) == 0 {
		problems = append(problems, ErrEmptyPayload)
	}
	if len(problems) > 0 {
		return WebhookEvent{}, errors.Join(problems...)
	}

	return WebhookEvent{
		ProjectID:   projectID,
		EventType:   eventType,
		DeliveryKey: deliveryKey,
		Payload:     payload,
		Status:      WebhookStatusReceived,
	}, nil
}
