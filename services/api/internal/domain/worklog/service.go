package worklogdom

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Service defines task worklog use cases.
type Service interface {
	// ListWorklogs returns all worklogs for taskID, verifying the task belongs to projectID.
	ListWorklogs(ctx context.Context, projectID, taskID uuid.UUID) ([]*Worklog, error)
	CreateWorklog(ctx context.Context, in CreateWorklogInput) (*Worklog, error)
	// DeleteWorklog removes worklogID, verifying it belongs to taskID within projectID.
	DeleteWorklog(ctx context.Context, projectID, taskID, worklogID uuid.UUID) error
}

// CreateWorklogInput carries fields required to log work against a task.
// LoggedAt is optional and defaults to the current time when nil.
type CreateWorklogInput struct {
	ProjectID uuid.UUID
	TaskID    uuid.UUID
	MemberID  *uuid.UUID
	Minutes   int
	Note      string
	LoggedAt  *time.Time
}

// TaskOwnerChecker verifies that a task belongs to a given project. It lets the
// worklog service scope operations by project without depending on the task
// service or aggregate.
type TaskOwnerChecker interface {
	TaskBelongsToProject(ctx context.Context, projectID, taskID uuid.UUID) error
}
