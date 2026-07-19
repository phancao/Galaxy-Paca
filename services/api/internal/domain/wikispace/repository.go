package wikispacedom

import (
	"context"

	"github.com/google/uuid"
)

// Repository persists the Paca-side Wiki associations.
type Repository interface {
	// GetSpace returns the project's wiki space mapping, ErrSpaceNotFound
	// when the project has not been provisioned yet.
	GetSpace(ctx context.Context, projectID uuid.UUID) (*ProjectWikiSpace, error)
	// SaveSpace upserts the project's wiki space mapping.
	SaveSpace(ctx context.Context, s *ProjectWikiSpace) error
	// ListTaskLinks returns a task's wiki links, newest first.
	ListTaskLinks(ctx context.Context, taskID uuid.UUID) ([]*TaskWikiLink, error)
	// AddTaskLink upserts a task↔record link (title/url refresh on conflict).
	AddTaskLink(ctx context.Context, l *TaskWikiLink) error
	// RemoveTaskLink deletes one link; ErrLinkNotFound when absent.
	RemoveTaskLink(ctx context.Context, taskID uuid.UUID, wikiRecordID string) error
}
