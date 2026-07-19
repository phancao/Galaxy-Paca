package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Paca-AI/api/internal/apierr"
	projectdom "github.com/Paca-AI/api/internal/domain/project"
	worklogdom "github.com/Paca-AI/api/internal/domain/worklog"
	"github.com/Paca-AI/api/internal/transport/http/dto"
	"github.com/Paca-AI/api/internal/transport/http/middleware"
	"github.com/Paca-AI/api/internal/transport/http/presenter"
)

// WorklogHandler handles task worklog (time-tracking) endpoints.
type WorklogHandler struct {
	svc        worklogdom.Service
	memberRepo projectdom.MemberRepository
}

// NewWorklogHandler returns a WorklogHandler wired to the worklog service.
func NewWorklogHandler(svc worklogdom.Service) *WorklogHandler {
	return &WorklogHandler{svc: svc}
}

// WithMemberRepo attaches the project member repository used to attribute a new
// worklog to the authenticated caller's project_members.id (see resolveMemberID).
func (h *WorklogHandler) WithMemberRepo(repo projectdom.MemberRepository) *WorklogHandler {
	h.memberRepo = repo
	return h
}

// resolveMemberID best-effort maps the authenticated caller's user ID to their
// project_members.id within projectID, using the same claims subject +
// FindMemberByUserProject lookup as AgentHandler.resolveMemberID. It returns nil
// (rather than an error) when the caller is not a project member or cannot be
// resolved, so the worklog is still recorded with a NULL member attribution.
func (h *WorklogHandler) resolveMemberID(r *http.Request, projectID uuid.UUID) *uuid.UUID {
	if h.memberRepo == nil {
		return nil
	}
	claims := middleware.ClaimsFrom(r)
	if claims == nil {
		return nil
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil
	}
	member, err := h.memberRepo.FindMemberByUserProject(r.Context(), userID, projectID)
	if err != nil {
		return nil
	}
	return &member.ID
}

// ListWorklogs handles GET /projects/:projectId/tasks/:taskId/worklogs.
func (h *WorklogHandler) ListWorklogs(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	taskID, err := parseTaskID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}

	worklogs, err := h.svc.ListWorklogs(r.Context(), projectID, taskID)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	resp := make([]dto.WorklogResponse, 0, len(worklogs))
	totalMinutes := 0
	for _, wl := range worklogs {
		resp = append(resp, dto.WorklogFromEntity(wl))
		totalMinutes += wl.Minutes
	}
	presenter.OK(w, r, map[string]any{"items": resp, "total_minutes": totalMinutes})
}

// ListProjectWorklogs handles GET /projects/:projectId/worklogs. It returns every
// worklog across the project's tasks, filtered by the optional query params
// `from` / `to` (RFC3339 timestamps, inclusive) and `member_id`. Powers the team
// efficiency / time-tracking reports.
func (h *WorklogHandler) ListProjectWorklogs(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	q := r.URL.Query()
	var filter worklogdom.WorklogFilter
	if v := strings.TrimSpace(q.Get("from")); v != "" {
		if t, perr := time.Parse(time.RFC3339, v); perr == nil {
			filter.From = &t
		}
	}
	if v := strings.TrimSpace(q.Get("to")); v != "" {
		if t, perr := time.Parse(time.RFC3339, v); perr == nil {
			filter.To = &t
		}
	}
	if v := strings.TrimSpace(q.Get("member_id")); v != "" {
		if id, perr := uuid.Parse(v); perr == nil {
			filter.MemberID = &id
		}
	}

	worklogs, err := h.svc.ListProjectWorklogs(r.Context(), projectID, filter)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	resp := make([]dto.WorklogResponse, 0, len(worklogs))
	totalMinutes := 0
	for _, wl := range worklogs {
		resp = append(resp, dto.WorklogFromEntity(wl))
		totalMinutes += wl.Minutes
	}
	presenter.OK(w, r, map[string]any{"items": resp, "total_minutes": totalMinutes})
}

// CreateWorklog handles POST /projects/:projectId/tasks/:taskId/worklogs.
func (h *WorklogHandler) CreateWorklog(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	taskID, err := parseTaskID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}

	var req dto.CreateWorklogRequest
	if !middleware.BindJSON(w, r, &req) {
		return
	}
	if req.Minutes <= 0 {
		presenter.Error(w, r, apierr.New(apierr.CodeBadRequest, "minutes must be a positive integer"))
		return
	}

	wl, err := h.svc.CreateWorklog(r.Context(), worklogdom.CreateWorklogInput{
		ProjectID: projectID,
		TaskID:    taskID,
		MemberID:  h.resolveMemberID(r, projectID),
		Minutes:   req.Minutes,
		Note:      req.Note,
		LoggedAt:  req.LoggedAt,
	})
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	presenter.Created(w, r, dto.WorklogFromEntity(wl))
}

// DeleteWorklog handles DELETE /projects/:projectId/tasks/:taskId/worklogs/:worklogId.
func (h *WorklogHandler) DeleteWorklog(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	taskID, err := parseTaskID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	worklogID, err := parseWorklogID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	if err := h.svc.DeleteWorklog(r.Context(), projectID, taskID, worklogID); err != nil {
		presenter.Error(w, r, err)
		return
	}
	presenter.NoContent(w)
}

func parseWorklogID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "worklogId"))
	if err != nil {
		return uuid.Nil, apierr.New(apierr.CodeBadRequest, "invalid worklog id")
	}
	return id, nil
}
