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

	incidents, err := h.deps.Incidents.List(r.Context(), OwnerIDFromContext(r.Context()), gitlabID, limit)
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

// occurrenceResponse is the JSON shape of a single enriched failure occurrence.
type occurrenceResponse struct {
	ObservedAt       time.Time `json:"observed_at"`
	Ref              string    `json:"ref"`
	SHA              string    `json:"sha"`
	JobName          string    `json:"job_name"`
	Stage            string    `json:"stage"`
	GitLabJobID      int64     `json:"gitlab_job_id"`
	GitLabPipelineID int64     `json:"gitlab_pipeline_id"`
	EvidenceExcerpt  string    `json:"evidence_excerpt"`
}

// refSummaryResponse is the JSON shape of a per-ref occurrence breakdown.
type refSummaryResponse struct {
	Ref             string `json:"ref"`
	OccurrenceCount int    `json:"occurrence_count"`
	DistinctSHAs    int    `json:"distinct_shas"`
}

// signalsResponse is the JSON shape of the heuristic incident signals.
type signalsResponse struct {
	DistinctRefs         int  `json:"distinct_refs"`
	DistinctSHAs         int  `json:"distinct_shas"`
	RepeatedOnSameCommit bool `json:"repeated_on_same_commit"`
	SpansMultipleCommits bool `json:"spans_multiple_commits"`
	FlakySuspected       bool `json:"flaky_suspected"`
	RegressionSuspected  bool `json:"regression_suspected"`
}

// incidentDetailResponse is the JSON shape of a single incident with its
// occurrence history, per-ref breakdown and heuristic signals.
type incidentDetailResponse struct {
	incidentResponse
	Occurrences  []occurrenceResponse `json:"occurrences"`
	AffectedRefs []refSummaryResponse `json:"affected_refs"`
	Signals      signalsResponse      `json:"signals"`
}

// getIncident serves
// GET /api/v1/projects/{gitlab_project_id}/incidents/{fingerprint}.
func (h *apiHandlers) getIncident(w http.ResponseWriter, r *http.Request) {
	gitlabID, err := strconv.ParseInt(r.PathValue("gitlab_project_id"), 10, 64)
	if err != nil || gitlabID <= 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid gitlab_project_id"})
		return
	}

	fingerprint := r.PathValue("fingerprint")
	if fingerprint == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "missing fingerprint"})
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit")) // 0 (and negatives) → service default

	detail, err := h.deps.Incidents.Detail(r.Context(), OwnerIDFromContext(r.Context()), gitlabID, fingerprint, limit)
	if err != nil {
		switch {
		case errors.Is(err, application.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "project not registered"})
		case errors.Is(err, application.ErrIncidentNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "incident not found"})
		default:
			h.logger.Error("get incident failed", slog.String("error", err.Error()))
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
		}
		return
	}

	writeJSON(w, http.StatusOK, toIncidentDetailResponse(detail))
}

func toIncidentDetailResponse(d domain.IncidentDetail) incidentDetailResponse {
	occ := make([]occurrenceResponse, 0, len(d.Occurrences))
	for _, o := range d.Occurrences {
		occ = append(occ, occurrenceResponse{
			ObservedAt:       o.ObservedAt,
			Ref:              o.Ref,
			SHA:              o.SHA,
			JobName:          o.JobName,
			Stage:            o.Stage,
			GitLabJobID:      o.GitLabJobID,
			GitLabPipelineID: o.GitLabPipelineID,
			EvidenceExcerpt:  o.EvidenceExcerpt,
		})
	}
	refs := make([]refSummaryResponse, 0, len(d.AffectedRefs))
	for _, rs := range d.AffectedRefs {
		refs = append(refs, refSummaryResponse{
			Ref:             rs.Ref,
			OccurrenceCount: rs.OccurrenceCount,
			DistinctSHAs:    rs.DistinctSHAs,
		})
	}
	return incidentDetailResponse{
		incidentResponse: toIncidentResponse(d.Incident),
		Occurrences:      occ,
		AffectedRefs:     refs,
		Signals: signalsResponse{
			DistinctRefs:         d.Signals.DistinctRefs,
			DistinctSHAs:         d.Signals.DistinctSHAs,
			RepeatedOnSameCommit: d.Signals.RepeatedOnSameCommit,
			SpansMultipleCommits: d.Signals.SpansMultipleCommits,
			FlakySuspected:       d.Signals.FlakySuspected,
			RegressionSuspected:  d.Signals.RegressionSuspected,
		},
	}
}
