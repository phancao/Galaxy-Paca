package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Paca-AI/api/internal/apierr"
	wikispacedom "github.com/Paca-AI/api/internal/domain/wikispace"
	"github.com/Paca-AI/api/internal/transport/http/middleware"
	"github.com/Paca-AI/api/internal/transport/http/presenter"
)

// WikiHandler exposes the ADR-042 Wiki-backed Documentation surface: the
// project's wiki space (embedded in the Documentation tab), a server-side
// search/tree proxy (the browser cannot call the Wiki API cross-origin), and
// task↔page links.
type WikiHandler struct {
	svc wikiSpaceService
}

// wikiSpaceService is the service surface the handler needs
// (wikispacedom.Service plus the enablement probe).
type wikiSpaceService interface {
	wikispacedom.Service
	Enabled() bool
}

// NewWikiHandler returns a WikiHandler wired to the wikispace service.
func NewWikiHandler(svc wikiSpaceService) *WikiHandler {
	return &WikiHandler{svc: svc}
}

// Enabled reports whether the Wiki integration is configured (used by the
// router to register routes only when live).
func (h *WikiHandler) Enabled() bool { return h != nil && h.svc != nil && h.svc.Enabled() }

// actorUserID resolves the authenticated caller's user id (uuid.Nil when
// unresolvable — the service then acts as the plain service account).
func (h *WikiHandler) actorUserID(r *http.Request) uuid.UUID {
	claims := middleware.ClaimsFrom(r)
	if claims == nil {
		return uuid.Nil
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil
	}
	return id
}

type wikiSpaceDTO struct {
	FolderID string `json:"folder_id"`
	URL      string `json:"url"`
	Created  bool   `json:"created"`
}

type wikiPageDTO struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	URL      string        `json:"url"`
	Context  string        `json:"context,omitempty"`
	Children []wikiPageDTO `json:"children,omitempty"`
}

type wikiLinkDTO struct {
	ID       string `json:"id"`
	RecordID string `json:"record_id"`
	URL      string `json:"url"`
	Title    string `json:"title"`
}

func toWikiSpaceDTO(s *wikispacedom.SpaceInfo) wikiSpaceDTO {
	return wikiSpaceDTO{FolderID: s.FolderID, URL: s.URL, Created: s.Created}
}

func toWikiPageDTOs(nodes []wikispacedom.PageNode) []wikiPageDTO {
	out := make([]wikiPageDTO, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, wikiPageDTO{
			ID:       n.ID,
			Title:    n.Title,
			URL:      n.URL,
			Children: toWikiPageDTOs(n.Children),
		})
	}
	return out
}

func toWikiLinkDTO(l *wikispacedom.TaskWikiLink) wikiLinkDTO {
	return wikiLinkDTO{ID: l.ID.String(), RecordID: l.WikiRecordID, URL: l.WikiURL, Title: l.Title}
}

// GetSpace ensures + returns the project's wiki space.
// GET /projects/{projectId}/wiki-space
func (h *WikiHandler) GetSpace(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	space, err := h.svc.EnsureSpace(r.Context(), projectID, h.actorUserID(r))
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	presenter.OK(w, r, toWikiSpaceDTO(space))
}

// GetTree returns the space plus its page tree.
// GET /projects/{projectId}/wiki-space/tree
func (h *WikiHandler) GetTree(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	space, nodes, err := h.svc.Tree(r.Context(), projectID, h.actorUserID(r))
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	presenter.OK(w, r, map[string]any{
		"space": toWikiSpaceDTO(space),
		"items": toWikiPageDTOs(nodes),
	})
}

// Search full-text searches the project's space.
// GET /projects/{projectId}/wiki-space/search?q=
func (h *WikiHandler) Search(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		presenter.Error(w, r, apierr.New(apierr.CodeBadRequest, "q is required"))
		return
	}
	hits, err := h.svc.Search(r.Context(), projectID, h.actorUserID(r), query)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	items := make([]wikiPageDTO, 0, len(hits))
	for _, hit := range hits {
		items = append(items, wikiPageDTO{ID: hit.ID, Title: hit.Title, URL: hit.URL, Context: hit.Context})
	}
	presenter.OK(w, r, map[string]any{"items": items})
}

// CreatePage creates an empty published page in the project's space.
// POST /projects/{projectId}/wiki-space/pages {"title": "..."}
func (h *WikiHandler) CreatePage(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		presenter.Error(w, r, apierr.New(apierr.CodeBadRequest, "invalid JSON body"))
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		presenter.Error(w, r, apierr.New(apierr.CodeBadRequest, "title is required"))
		return
	}
	page, err := h.svc.CreatePage(r.Context(), projectID, h.actorUserID(r), title)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	presenter.Created(w, r, wikiPageDTO{ID: page.ID, Title: page.Title, URL: page.URL})
}

// ListTaskLinks returns a task's linked wiki pages.
// GET /projects/{projectId}/tasks/{taskId}/wiki-links
func (h *WikiHandler) ListTaskLinks(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseParamUUID(r, "taskId")
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	links, err := h.svc.ListTaskLinks(r.Context(), taskID)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	items := make([]wikiLinkDTO, 0, len(links))
	for _, l := range links {
		items = append(items, toWikiLinkDTO(l))
	}
	presenter.OK(w, r, map[string]any{"items": items})
}

// AddTaskLink links a wiki record to a task.
// POST /projects/{projectId}/tasks/{taskId}/wiki-links {"record_id": "..."}
func (h *WikiHandler) AddTaskLink(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseParamUUID(r, "taskId")
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	var body struct {
		RecordID string `json:"record_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		presenter.Error(w, r, apierr.New(apierr.CodeBadRequest, "invalid JSON body"))
		return
	}
	recordID := strings.TrimSpace(body.RecordID)
	if recordID == "" {
		presenter.Error(w, r, apierr.New(apierr.CodeBadRequest, "record_id is required"))
		return
	}
	link, err := h.svc.AddTaskLink(r.Context(), taskID, h.actorUserID(r), recordID)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	presenter.Created(w, r, toWikiLinkDTO(link))
}

// RemoveTaskLink unlinks a wiki record from a task.
// DELETE /projects/{projectId}/tasks/{taskId}/wiki-links/{recordId}
func (h *WikiHandler) RemoveTaskLink(w http.ResponseWriter, r *http.Request) {
	taskID, err := parseParamUUID(r, "taskId")
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	recordID := strings.TrimSpace(chi.URLParam(r, "recordId"))
	if recordID == "" {
		presenter.Error(w, r, apierr.New(apierr.CodeBadRequest, "recordId is required"))
		return
	}
	if err := h.svc.RemoveTaskLink(r.Context(), taskID, recordID); err != nil {
		presenter.Error(w, r, err)
		return
	}
	presenter.NoContent(w)
}
