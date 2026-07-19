package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Paca-AI/api/internal/apierr"
	componentdom "github.com/Paca-AI/api/internal/domain/component"
	"github.com/Paca-AI/api/internal/transport/http/dto"
	"github.com/Paca-AI/api/internal/transport/http/middleware"
	"github.com/Paca-AI/api/internal/transport/http/presenter"
)

// ComponentHandler handles project component endpoints.
type ComponentHandler struct {
	svc componentdom.Service
}

// NewComponentHandler returns a ComponentHandler wired to the component service.
func NewComponentHandler(svc componentdom.Service) *ComponentHandler {
	return &ComponentHandler{svc: svc}
}

// ListComponents handles GET /projects/:projectId/components.
func (h *ComponentHandler) ListComponents(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	components, err := h.svc.ListComponents(r.Context(), projectID)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	resp := make([]dto.ComponentResponse, 0, len(components))
	for _, c := range components {
		resp = append(resp, dto.ComponentFromEntity(c))
	}
	presenter.OK(w, r, map[string]any{"items": resp})
}

// CreateComponent handles POST /projects/:projectId/components.
func (h *ComponentHandler) CreateComponent(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}

	var req dto.CreateComponentRequest
	if !middleware.BindJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		presenter.Error(w, r, apierr.New(apierr.CodeBadRequest, "name is required"))
		return
	}

	c, err := h.svc.CreateComponent(r.Context(), componentdom.CreateComponentInput{
		ProjectID:    projectID,
		Name:         req.Name,
		Description:  req.Description,
		LeadMemberID: req.LeadMemberID,
	})
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	presenter.Created(w, r, dto.ComponentFromEntity(c))
}

// UpdateComponent handles PATCH /projects/:projectId/components/:componentId.
func (h *ComponentHandler) UpdateComponent(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	componentID, err := parseComponentID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}

	var req dto.UpdateComponentRequest
	if !middleware.BindJSON(w, r, &req) {
		return
	}

	c, err := h.svc.UpdateComponent(r.Context(), projectID, componentID, componentdom.UpdateComponentInput{
		Name:         req.Name,
		Description:  req.Description.Ptr(),
		LeadMemberID: req.LeadMemberID.Ptr(),
	})
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	presenter.OK(w, r, dto.ComponentFromEntity(c))
}

// DeleteComponent handles DELETE /projects/:projectId/components/:componentId.
func (h *ComponentHandler) DeleteComponent(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	componentID, err := parseComponentID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	if err := h.svc.DeleteComponent(r.Context(), projectID, componentID); err != nil {
		presenter.Error(w, r, err)
		return
	}
	presenter.OK(w, r, map[string]any{"message": "component deleted"})
}

func parseComponentID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "componentId"))
	if err != nil {
		return uuid.Nil, apierr.New(apierr.CodeBadRequest, "invalid component id")
	}
	return id, nil
}
