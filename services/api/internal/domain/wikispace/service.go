package wikispacedom

import (
	"context"

	"github.com/google/uuid"
)

// SpaceInfo is the provisioned wiki space of a project, as consumed by the
// Documentation surface (URL is absolute, browser-facing).
type SpaceInfo struct {
	ProjectID uuid.UUID
	FolderID  string
	URL       string
	// Created reports whether this call provisioned the space.
	Created bool
}

// PageRef references one Wiki page with its browser-facing URL.
type PageRef struct {
	ID    string
	Title string
	URL   string
}

// PageNode is one node of the space's page tree.
type PageNode struct {
	ID       string
	Title    string
	URL      string
	Children []PageNode
}

// SearchHit is one full-text search result within the project's space.
type SearchHit struct {
	PageRef
	Context string
}

// Service orchestrates the Wiki-backed Documentation surface (ADR-042):
// lazy space provisioning via act-as, page tree/search proxying (the browser
// cannot call the Wiki API cross-origin), and task↔page links.
type Service interface {
	// EnsureSpace returns the project's wiki space, provisioning the Wiki
	// Folder on first use (acting as the requesting user).
	EnsureSpace(ctx context.Context, projectID, actorUserID uuid.UUID) (*SpaceInfo, error)
	// Tree returns the space plus its page tree.
	Tree(ctx context.Context, projectID, actorUserID uuid.UUID) (*SpaceInfo, []PageNode, error)
	// Search full-text searches within the project's space.
	Search(ctx context.Context, projectID, actorUserID uuid.UUID, query string) ([]SearchHit, error)
	// CreatePage creates an empty published page in the project's space.
	CreatePage(ctx context.Context, projectID, actorUserID uuid.UUID, title string) (*PageRef, error)
	// ListTaskLinks returns a task's linked wiki pages.
	ListTaskLinks(ctx context.Context, taskID uuid.UUID) ([]*TaskWikiLink, error)
	// AddTaskLink links a wiki record to a task. The record is resolved (and
	// access-checked) against the Wiki acting as the requesting user.
	AddTaskLink(ctx context.Context, taskID, actorUserID uuid.UUID, wikiRecordID string) (*TaskWikiLink, error)
	// RemoveTaskLink unlinks a wiki record from a task.
	RemoveTaskLink(ctx context.Context, taskID uuid.UUID, wikiRecordID string) error
}
