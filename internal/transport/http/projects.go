package http

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/Jeston10/pipeline-sherlock/internal/application"
	"github.com/Jeston10/pipeline-sherlock/internal/domain"
)

// apiHandlers holds the dependencies shared by the v1 API handlers.
type apiHandlers struct {
	deps   Deps
	logger *slog.Logger
}

// errorResponse is the uniform error envelope. It carries a short, safe
// message only — never internal error detail.
type errorResponse struct {
	Error string `json:"error"`
}

type registerProjectRequest struct {
	GitLabProjectID int64  `json:"gitlab_project_id"`
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
}

type projectResponse struct {
	ID              string `json:"id"`
	GitLabProjectID int64  `json:"gitlab_project_id"`
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	// WebhookSecret is present only in the registration response (shown once).
	WebhookSecret string `json:"webhook_secret,omitempty"`
}

func (h *apiHandlers) registerProject(w http.ResponseWriter, r *http.Request) {
	var req registerProjectRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 64*1024))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}

	res, err := h.deps.Projects.Register(r.Context(), OwnerIDFromContext(r.Context()), req.GitLabProjectID, req.Name, req.Namespace)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidProject):
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid project fields"})
		case errors.Is(err, application.ErrProjectAlreadyExists):
			writeJSON(w, http.StatusConflict, errorResponse{Error: "project already registered"})
		default:
			h.logger.Error("register project failed", slog.String("error", err.Error()))
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
		}
		return
	}

	writeJSON(w, http.StatusCreated, projectResponse{
		ID:              string(res.Project.ID),
		GitLabProjectID: res.Project.GitLabProjectID,
		Name:            res.Project.Name,
		Namespace:       res.Project.Namespace,
		WebhookSecret:   res.WebhookSecret,
	})
}

func (h *apiHandlers) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.deps.Projects.List(r.Context(), OwnerIDFromContext(r.Context()))
	if err != nil {
		h.logger.Error("list projects failed", slog.String("error", err.Error()))
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
		return
	}

	out := make([]projectResponse, 0, len(projects))
	for _, p := range projects {
		// Never expose the webhook secret (or its hash) on list.
		out = append(out, projectResponse{
			ID:              string(p.ID),
			GitLabProjectID: p.GitLabProjectID,
			Name:            p.Name,
			Namespace:       p.Namespace,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
