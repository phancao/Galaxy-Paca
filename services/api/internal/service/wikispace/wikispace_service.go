// Package wikispace implements the ADR-042 Wiki-backed Documentation surface:
// one Galaxy AI Wiki Folder (space) per project, provisioned lazily via the
// platform act-as pattern, plus task↔page links. The Wiki owns all document
// content and permissions; this service only orchestrates and stores the
// Paca-side associations.
package wikispace

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	projectdom "github.com/Paca-AI/api/internal/domain/project"
	userdom "github.com/Paca-AI/api/internal/domain/user"
	wikispacedom "github.com/Paca-AI/api/internal/domain/wikispace"
	"github.com/Paca-AI/api/internal/platform/wiki"
	"github.com/google/uuid"
)

// Service implements wikispacedom.Service.
type Service struct {
	repo     wikispacedom.Repository
	wiki     *wiki.Client
	projects projectdom.Repository
	members  projectdom.MemberRepository
	users    userdom.Repository
	log      *slog.Logger
}

// New returns a Service. A nil/disabled wiki client makes every method
// return wikispacedom.ErrDisabled.
func New(
	repo wikispacedom.Repository,
	wikiClient *wiki.Client,
	projects projectdom.Repository,
	members projectdom.MemberRepository,
	users userdom.Repository,
	log *slog.Logger,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{repo: repo, wiki: wikiClient, projects: projects, members: members, users: users, log: log}
}

// Enabled reports whether the Wiki integration is configured.
func (s *Service) Enabled() bool { return s.wiki.Enabled() }

// visibilityFromPermission maps a Wiki folder permission to the domain
// visibility value.
func visibilityFromPermission(permission *string) string {
	switch {
	case permission == nil:
		return wikispacedom.VisibilityPrivate
	case *permission == "read":
		return wikispacedom.VisibilityTeamRead
	case *permission == "read_write":
		return wikispacedom.VisibilityTeamWrite
	default:
		return ""
	}
}

// permissionFromVisibility maps a domain visibility value to the Wiki folder
// permission; ok=false for unknown values.
func permissionFromVisibility(visibility string) (perm *string, ok bool) {
	switch visibility {
	case wikispacedom.VisibilityPrivate:
		return nil, true
	case wikispacedom.VisibilityTeamRead:
		v := "read"
		return &v, true
	case wikispacedom.VisibilityTeamWrite:
		v := "read_write"
		return &v, true
	default:
		return nil, false
	}
}

// actorFor resolves a Paca user to the Wiki act-as actor. Users without an
// OIDC identity (local accounts) act as the plain service account.
func (s *Service) actorFor(ctx context.Context, userID uuid.UUID) (wiki.Actor, *userdom.User) {
	if userID == uuid.Nil {
		return wiki.Actor{}, nil
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil || u == nil {
		return wiki.Actor{}, nil
	}
	return wiki.Actor{Sub: u.OIDCSub, Email: u.Email}, u
}

// EnsureSpace returns the project's wiki space, provisioning on first use.
func (s *Service) EnsureSpace(ctx context.Context, projectID, actorUserID uuid.UUID) (*wikispacedom.SpaceInfo, error) {
	if !s.wiki.Enabled() {
		return nil, wikispacedom.ErrDisabled
	}
	if existing, err := s.repo.GetSpace(ctx, projectID); err == nil {
		info := &wikispacedom.SpaceInfo{
			ProjectID: projectID,
			FolderID:  existing.WikiFolderID,
			URL:       existing.WikiURL,
		}
		// Visibility is read live from the Wiki, best-effort: the surface
		// must keep working (URL from the mapping) when the Wiki hiccups.
		actor, _ := s.actorFor(ctx, actorUserID)
		if folder, ferr := s.wiki.FolderInfo(ctx, actor, existing.WikiFolderID); ferr == nil {
			info.Visibility = visibilityFromPermission(folder.Permission)
		}
		return info, nil
	} else if err != wikispacedom.ErrSpaceNotFound {
		return nil, err
	}

	project, err := s.projects.FindByID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	actor, _ := s.actorFor(ctx, actorUserID)

	// Open (public) projects get a team-visible read_write space; private
	// projects get a membership-only folder mirrored from project members.
	folder, err := s.wiki.CreateFolder(ctx, actor,
		project.Name,
		fmt.Sprintf("Paca project documentation — %s", project.TaskIDPrefix),
		!project.IsPublic)
	if err != nil {
		return nil, fmt.Errorf("wikispace: provision folder: %w", err)
	}

	space := &wikispacedom.ProjectWikiSpace{
		ProjectID:    projectID,
		WikiFolderID: folder.ID,
		WikiURL:      s.wiki.PublicPageURL(folder.URL),
	}
	if actorUserID != uuid.Nil {
		space.CreatedBy = &actorUserID
	}
	if err := s.repo.SaveSpace(ctx, space); err != nil {
		return nil, err
	}

	// Membership mirroring is best-effort and asynchronous: a Wiki-side
	// failure must never fail the Documentation surface. Private folders
	// need explicit grants; open folders are team-visible already.
	if !project.IsPublic {
		s.syncMembersAsync(projectID, folder.ID, actor)
	}

	visibility := wikispacedom.VisibilityTeamWrite
	if !project.IsPublic {
		visibility = wikispacedom.VisibilityPrivate
	}
	return &wikispacedom.SpaceInfo{
		ProjectID:  projectID,
		FolderID:   folder.ID,
		URL:        space.WikiURL,
		Created:    true,
		Visibility: visibility,
	}, nil
}

// SetSpaceVisibility changes the space's team-wide access, acting as the
// requesting user (the Wiki enforces its own folder-admin authorization).
func (s *Service) SetSpaceVisibility(ctx context.Context, projectID, actorUserID uuid.UUID, visibility string) (*wikispacedom.SpaceInfo, error) {
	if !s.wiki.Enabled() {
		return nil, wikispacedom.ErrDisabled
	}
	perm, ok := permissionFromVisibility(visibility)
	if !ok {
		return nil, wikispacedom.ErrVisibilityInvalid
	}
	space, err := s.EnsureSpace(ctx, projectID, actorUserID)
	if err != nil {
		return nil, err
	}
	actor, _ := s.actorFor(ctx, actorUserID)
	folder, err := s.wiki.UpdateFolderPermission(ctx, actor, space.FolderID, perm)
	if err != nil {
		return nil, fmt.Errorf("wikispace: set visibility: %w", err)
	}
	// Going private cuts off tenant-wide access — mirror the project's
	// members onto the folder so they keep theirs (best-effort, async).
	if visibility == wikispacedom.VisibilityPrivate {
		s.syncMembersAsync(projectID, space.FolderID, actor)
	}
	space.Visibility = visibilityFromPermission(folder.Permission)
	return space, nil
}

// syncMembersAsync mirrors current project members onto a private folder.
// Members who never signed in to the Wiki are skipped (they get access on
// the next sync after their first login).
func (s *Service) syncMembersAsync(projectID uuid.UUID, folderID string, actor wiki.Actor) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		members, err := s.members.ListMembers(ctx, projectID)
		if err != nil {
			s.log.Warn("wikispace: member sync: list members", "project_id", projectID, "error", err)
			return
		}
		for _, m := range members {
			if m.UserID == uuid.Nil { // agent members have no wiki identity
				continue
			}
			u, err := s.users.FindByID(ctx, m.UserID)
			if err != nil || u == nil || u.Email == "" {
				continue
			}
			wikiUserID, err := s.wiki.ResolveUserByEmail(ctx, u.Email)
			if err != nil || wikiUserID == "" {
				continue
			}
			if err := s.wiki.AddFolderUser(ctx, actor, folderID, wikiUserID, "read_write"); err != nil {
				s.log.Warn("wikispace: member sync: grant", "project_id", projectID, "email", u.Email, "error", err)
			}
		}
	}()
}

// Tree returns the space plus its page tree.
func (s *Service) Tree(ctx context.Context, projectID, actorUserID uuid.UUID) (*wikispacedom.SpaceInfo, []wikispacedom.PageNode, error) {
	space, err := s.EnsureSpace(ctx, projectID, actorUserID)
	if err != nil {
		return nil, nil, err
	}
	actor, _ := s.actorFor(ctx, actorUserID)
	nodes, err := s.wiki.FolderRecords(ctx, actor, space.FolderID)
	if err != nil {
		return nil, nil, fmt.Errorf("wikispace: page tree: %w", err)
	}
	return space, s.toPageNodes(nodes), nil
}

func (s *Service) toPageNodes(nodes []wiki.NavNode) []wikispacedom.PageNode {
	out := make([]wikispacedom.PageNode, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, wikispacedom.PageNode{
			ID:       n.ID,
			Title:    n.Title,
			URL:      s.wiki.PublicPageURL(n.URL),
			Children: s.toPageNodes(n.Children),
		})
	}
	return out
}

// Search full-text searches within the project's space.
func (s *Service) Search(ctx context.Context, projectID, actorUserID uuid.UUID, query string) ([]wikispacedom.SearchHit, error) {
	space, err := s.EnsureSpace(ctx, projectID, actorUserID)
	if err != nil {
		return nil, err
	}
	actor, _ := s.actorFor(ctx, actorUserID)
	results, err := s.wiki.SearchRecords(ctx, actor, query, space.FolderID)
	if err != nil {
		return nil, fmt.Errorf("wikispace: search: %w", err)
	}
	hits := make([]wikispacedom.SearchHit, 0, len(results))
	for _, r := range results {
		hits = append(hits, wikispacedom.SearchHit{
			PageRef: wikispacedom.PageRef{
				ID:    r.Record.ID,
				Title: r.Record.Title,
				URL:   s.wiki.PublicPageURL(r.Record.URL),
			},
			Context: r.Context,
		})
	}
	return hits, nil
}

// CreatePage creates an empty published page in the project's space.
func (s *Service) CreatePage(ctx context.Context, projectID, actorUserID uuid.UUID, title string) (*wikispacedom.PageRef, error) {
	space, err := s.EnsureSpace(ctx, projectID, actorUserID)
	if err != nil {
		return nil, err
	}
	actor, _ := s.actorFor(ctx, actorUserID)
	rec, err := s.wiki.CreateRecord(ctx, actor, space.FolderID, title, "")
	if err != nil {
		return nil, fmt.Errorf("wikispace: create page: %w", err)
	}
	return &wikispacedom.PageRef{
		ID:    rec.ID,
		Title: rec.Title,
		URL:   s.wiki.PublicPageURL(rec.URL),
	}, nil
}

// ListTaskLinks returns a task's linked wiki pages.
func (s *Service) ListTaskLinks(ctx context.Context, taskID uuid.UUID) ([]*wikispacedom.TaskWikiLink, error) {
	if !s.wiki.Enabled() {
		return nil, wikispacedom.ErrDisabled
	}
	return s.repo.ListTaskLinks(ctx, taskID)
}

// AddTaskLink links a wiki record to a task, resolving (and access-checking)
// it against the Wiki as the requesting user.
func (s *Service) AddTaskLink(ctx context.Context, taskID, actorUserID uuid.UUID, wikiRecordID string) (*wikispacedom.TaskWikiLink, error) {
	if !s.wiki.Enabled() {
		return nil, wikispacedom.ErrDisabled
	}
	actor, _ := s.actorFor(ctx, actorUserID)
	rec, err := s.wiki.RecordInfo(ctx, actor, wikiRecordID)
	if err != nil {
		return nil, fmt.Errorf("wikispace: resolve record: %w", err)
	}
	link := &wikispacedom.TaskWikiLink{
		TaskID:       taskID,
		WikiRecordID: rec.ID,
		WikiURL:      s.wiki.PublicPageURL(rec.URL),
		Title:        rec.Title,
	}
	if actorUserID != uuid.Nil {
		link.CreatedBy = &actorUserID
	}
	if err := s.repo.AddTaskLink(ctx, link); err != nil {
		return nil, err
	}
	return link, nil
}

// RemoveTaskLink unlinks a wiki record from a task.
func (s *Service) RemoveTaskLink(ctx context.Context, taskID uuid.UUID, wikiRecordID string) error {
	if !s.wiki.Enabled() {
		return wikispacedom.ErrDisabled
	}
	return s.repo.RemoveTaskLink(ctx, taskID, wikiRecordID)
}
