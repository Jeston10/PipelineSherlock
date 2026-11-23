package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Jeston10/pipeline-sherlock/internal/application"
	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// incidentResponse is the JSON shape of a grouped failure incident.
type incidentResponse struct {
	Fingerprint         string    `json:"fingerprint"`
	Category            string    `json:"category"`
	NormalizedSignature string    `json:"normalized_signature"`
	OccurrenceCount     int64     `json:"occurrence_count"`
	AlgorithmVersion    int       `json:"algorithm_version"`
	FirstSeenAt         time.Time `json:"first_seen_at"`
	LastSeenAt          time.Time `json:"last_seen_at"`
}

// listIncidents serves GET /api/v1/projects/{gitlab_project_id}/incidents.
func (h *apiHandlers) listIncidents(w http.ResponseWriter, r *http.Request) {
	gitlabID, err := strconv.ParseInt(r.PathValue("gitlab_project_id"), 10, 64)
	if err != nil || gitlabID <= 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid gitlab_project_id"})
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit")) // 0 (and negatives) → service default

	incidents, err := h.deps.Incidents.List(r.Context(), gitlabID, limit)
	if err != nil {
		switch {
		case errors.Is(err, application.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "project not registered"})
		default:
			h.logger.Error("list incidents failed", slog.String("error", err.Error()))
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
		}
		return
	}

	out := make([]incidentResponse, 0, len(incidents))
	for _, inc := range incidents {
		out = append(out, toIncidentResponse(inc))
	}
	writeJSON(w, http.StatusOK, out)
}

func toIncidentResponse(inc domain.Incident) incidentResponse {
	return incidentResponse{
		Fingerprint:         inc.Fingerprint,
		Category:            inc.Category,
		NormalizedSignature: inc.NormalizedSignature,
		OccurrenceCount:     inc.OccurrenceCount,
		AlgorithmVersion:    inc.AlgorithmVersion,
		FirstSeenAt:         inc.FirstSeenAt,
		LastSeenAt:          inc.LastSeenAt,
	}
}
