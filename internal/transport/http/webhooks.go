package http

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/Jeston10/pipeline-sherlock/internal/application"
	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// webhookEnvelope is the minimal shape we parse from a GitLab webhook body to
// identify the project. The full payload is stored verbatim in the inbox for
// later processing.
type webhookEnvelope struct {
	ObjectKind string `json:"object_kind"`
	Project    struct {
		ID int64 `json:"id"`
	} `json:"project"`
}

type webhookResponse struct {
	Accepted  bool   `json:"accepted"`
	Duplicate bool   `json:"duplicate"`
	Status    string `json:"status"`
}

// gitlabWebhook validates and durably records an inbound GitLab webhook. It
// acknowledges quickly after the durable write; actual processing happens
// asynchronously (later milestone).
func (h *apiHandlers) gitlabWebhook(w http.ResponseWriter, r *http.Request) {
	// Bound the body size before reading anything.
	r.Body = http.MaxBytesReader(w, r.Body, h.deps.maxWebhookBytes())
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "request body too large"})
		return
	}

	eventType := r.Header.Get("X-Gitlab-Event")
	if eventType == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "missing X-Gitlab-Event header"})
		return
	}

	var env webhookEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}
	if env.Project.ID <= 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "missing project.id in payload"})
		return
	}

	cmd := application.IngestCommand{
		GitLabProjectID: env.Project.ID,
		Token:           r.Header.Get("X-Gitlab-Token"),
		EventType:       eventType,
		DeliveryKey:     deliveryKey(r, body),
		Payload:         body,
	}

	res, err := h.deps.Webhooks.Ingest(r.Context(), cmd)
	if err != nil {
		h.writeIngestError(w, err)
		return
	}

	status := "accepted"
	if !res.Accepted {
		status = "duplicate"
	}
	writeJSON(w, http.StatusAccepted, webhookResponse{
		Accepted:  res.Accepted,
		Duplicate: !res.Accepted,
		Status:    status,
	})
}

func (h *apiHandlers) writeIngestError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidSignature):
		// Do not reveal whether the project exists on an auth failure.
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid webhook token"})
	case errors.Is(err, application.ErrProjectNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "unknown project"})
	case errors.Is(err, domain.ErrInvalidWebhookEvent):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid webhook event"})
	default:
		h.logger.Error("ingest webhook failed", slog.String("error", err.Error()))
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
	}
}

// deliveryKey prefers GitLab's delivery UUID header; when absent it falls back
// to a content hash so identical redeliveries still de-duplicate.
func deliveryKey(r *http.Request, body []byte) string {
	if uuid := r.Header.Get("X-Gitlab-Event-UUID"); uuid != "" {
		return uuid
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
