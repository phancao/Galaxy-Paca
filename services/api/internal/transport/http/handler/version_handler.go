package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Paca-AI/api/internal/apierr"
	versiondom "github.com/Paca-AI/api/internal/domain/version"
	"github.com/Paca-AI/api/internal/transport/http/dto"
	"github.com/Paca-AI/api/internal/transport/http/middleware"
	"github.com/Paca-AI/api/internal/transport/http/presenter"
)

// VersionHandler handles project version (fixVersion) endpoints.
type VersionHandler struct {
	svc versiondom.Service
}

// NewVersionHandler returns a VersionHandler wired to the version service.
func NewVersionHandler(svc versiondom.Service) *VersionHandler {
	return &VersionHandler{svc: svc}
}

// ListVersions handles GET /projects/:projectId/versions.
func (h *VersionHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	versions, err := h.svc.ListVersions(r.Context(), projectID)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	resp := make([]dto.VersionResponse, 0, len(versions))
	for _, v := range versions {
		resp = append(resp, dto.VersionFromEntity(v))
	}
	presenter.OK(w, r, map[string]any{"items": resp})
}

// CreateVersion handles POST /projects/:projectId/versions.
func (h *VersionHandler) CreateVersion(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}

	var req dto.CreateVersionRequest
	if !middleware.BindJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		presenter.Error(w, r, apierr.New(apierr.CodeBadRequest, "name is required"))
		return
	}

	v, err := h.svc.CreateVersion(r.Context(), versiondom.CreateVersionInput{
		ProjectID:   projectID,
		Name:        req.Name,
		Description: req.Description,
		Released:    req.Released,
		ReleaseDate: req.ReleaseDate,
		Archived:    req.Archived,
	})
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	presenter.Created(w, r, dto.VersionFromEntity(v))
}

// UpdateVersion handles PATCH /projects/:projectId/versions/:versionId.
func (h *VersionHandler) UpdateVersion(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	versionID, err := parseVersionID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}

	var req dto.UpdateVersionRequest
	if !middleware.BindJSON(w, r, &req) {
		return
	}

	v, err := h.svc.UpdateVersion(r.Context(), projectID, versionID, versiondom.UpdateVersionInput{
		Name:        req.Name,
		Description: req.Description.Ptr(),
		Released:    req.Released,
		ReleaseDate: req.ReleaseDate.Ptr(),
		Archived:    req.Archived,
	})
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	presenter.OK(w, r, dto.VersionFromEntity(v))
}

// DeleteVersion handles DELETE /projects/:projectId/versions/:versionId.
func (h *VersionHandler) DeleteVersion(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	versionID, err := parseVersionID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	if err := h.svc.DeleteVersion(r.Context(), projectID, versionID); err != nil {
		presenter.Error(w, r, err)
		return
	}
	presenter.OK(w, r, map[string]any{"message": "version deleted"})
}

func parseVersionID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "versionId"))
	if err != nil {
		return uuid.Nil, apierr.New(apierr.CodeBadRequest, "invalid version id")
	}
	return id, nil
}
