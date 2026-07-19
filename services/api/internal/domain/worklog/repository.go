package worklogdom

import (
	"context"

	"github.com/google/uuid"
)

// Repository is the persistence contract for the worklog aggregate.
type Repository interface {
	// ListWorklogs returns all worklogs for a task, ordered by logged_at ascending.
	ListWorklogs(ctx context.Context, taskID uuid.UUID) ([]*Worklog, error)
	// ListProjectWorklogs returns all worklogs across the project's tasks matching
	// filter, ordered by logged_at ascending.
	ListProjectWorklogs(ctx context.Context, projectID uuid.UUID, filter WorklogFilter) ([]*Worklog, error)
	FindWorklogByID(ctx context.Context, id uuid.UUID) (*Worklog, error)
	CreateWorklog(ctx context.Context, w *Worklog) error
	DeleteWorklog(ctx context.Context, id uuid.UUID) error
}
