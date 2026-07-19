package taskdom

import (
	"context"

	"github.com/google/uuid"
)

// Repository is the combined persistence contract for the task aggregate.
type Repository interface {
	TaskTypeRepository
	TaskStatusRepository
	TaskRepository
	TaskLinkRepository
	CustomFieldDefinitionRepository
}

// TaskLinkRepository defines persistence operations for task links.
type TaskLinkRepository interface {
	// ListTaskLinks returns all links where taskID is either source or target,
	// with LinkedTask populated. Links are ordered by created_at ascending.
	ListTaskLinks(ctx context.Context, taskID uuid.UUID) ([]*TaskLink, error)
	// FindTaskLinkByID returns a single link by primary key.
	FindTaskLinkByID(ctx context.Context, id uuid.UUID) (*TaskLink, error)
	// CreateTaskLinkIfNotExists persists l, unless an equivalent link already
	// exists (checked in both directions for relates_to), in which case it
	// returns created=false and no error. The existence check and insert run
	// in the same transaction with both task rows locked, so concurrent
	// attempts to link the same pair of tasks serialize instead of racing
	// past the duplicate check.
	CreateTaskLinkIfNotExists(ctx context.Context, l *TaskLink) (created bool, err error)
	// DeleteTaskLink removes the link identified by id.
	DeleteTaskLink(ctx context.Context, id uuid.UUID) error
}

// TaskTypeRepository defines persistence operations for task types.
type TaskTypeRepository interface {
	ListTaskTypes(ctx context.Context, projectID uuid.UUID) ([]*TaskType, error)
	FindTaskTypeByID(ctx context.Context, id uuid.UUID) (*TaskType, error)
	FindDefaultTaskType(ctx context.Context, projectID uuid.UUID) (*TaskType, error)
	CreateTaskType(ctx context.Context, t *TaskType) error
	UpdateTaskType(ctx context.Context, t *TaskType) error
	DeleteTaskType(ctx context.Context, id uuid.UUID) error
	// SetDefaultTaskType atomically clears is_default for every type in the
	// project and then marks the given type as the default.
	SetDefaultTaskType(ctx context.Context, projectID, typeID uuid.UUID) error
}

// TaskStatusRepository defines persistence operations for task statuses.
type TaskStatusRepository interface {
	ListTaskStatuses(ctx context.Context, projectID uuid.UUID) ([]*TaskStatus, error)
	FindTaskStatusByID(ctx context.Context, id uuid.UUID) (*TaskStatus, error)
	FindDefaultTaskStatus(ctx context.Context, projectID uuid.UUID) (*TaskStatus, error)
	CreateTaskStatus(ctx context.Context, s *TaskStatus) error
	UpdateTaskStatus(ctx context.Context, s *TaskStatus) error
	DeleteTaskStatus(ctx context.Context, id uuid.UUID) error
	// SetDefaultTaskStatus atomically clears is_default for every status in the
	// project and then marks the given status as the default.
	SetDefaultTaskStatus(ctx context.Context, projectID, statusID uuid.UUID) error
	// ReorderTaskStatuses atomically assigns position = index in statusIDs to
	// each status, verifying statusIDs is an exact permutation of the
	// project's existing statuses.
	ReorderTaskStatuses(ctx context.Context, projectID uuid.UUID, statusIDs []uuid.UUID) error
}

// TaskRepository defines persistence operations for tasks.
type TaskRepository interface {
	ListTasks(ctx context.Context, projectID uuid.UUID, filter TaskFilter, limit int, sort TaskSort) ([]*Task, bool, error)
	CountTasks(ctx context.Context, projectID uuid.UUID, filter TaskFilter) (int64, error)
	// SumTaskField sums a numeric field across all tasks matching the filter,
	// ignoring cursor-based pagination. fieldKey is "story_points" or a custom field key.
	SumTaskField(ctx context.Context, projectID uuid.UUID, filter TaskFilter, fieldKey string) (float64, error)
	FindTaskByID(ctx context.Context, id uuid.UUID) (*Task, error)
	FindTaskByNumber(ctx context.Context, projectID uuid.UUID, taskNumber int64) (*Task, error)
	CreateTask(ctx context.Context, t *Task) error
	UpdateTask(ctx context.Context, t *Task) error
	DeleteTask(ctx context.Context, id uuid.UUID) error
	// BulkMoveSprintTasks reassigns all non-done tasks in sourceSprintID to
	// targetSprintID. A nil targetSprintID moves tasks to the backlog (sprint_id = NULL).
	// Tasks whose status has category "done" are not moved.
	BulkMoveSprintTasks(ctx context.Context, projectID, sourceSprintID uuid.UUID, targetSprintID *uuid.UUID) error
}

// TaskSort carries resolved sort configuration for ListTasks.
// Kept separate from TaskFilter because sort order is a presentation concern,
// not a filter predicate.
//
// For built-in fields (importance, title, story_points, start_date, due_date,
// created) only By is set.  For custom field sorts, CFType and (for select)
// CFOpts are populated by the caller after looking up the field definition.
type TaskSort struct {
	By     string     // built-in field key, custom field key, or "" (default order)
	CFType string     // "number" | "date" | "select" — only for custom field sorts
	CFOpts []string   // ordered option values — only for "select" custom field sorts
	ViewID *uuid.UUID // when By == "view_position", JOIN view_task_positions for this view
}

// TaskFilter carries optional criteria for listing tasks.
type TaskFilter struct {
	SprintID     *uuid.UUID  // single-value compat; ignored when SprintIDs is non-empty
	SprintIDs    []uuid.UUID // multi-value; takes priority over SprintID and BacklogOnly
	StatusID     *uuid.UUID  // single-value compat; ignored when StatusIDs is non-empty
	StatusIDs    []uuid.UUID // multi-value; takes priority over StatusID
	AssigneeID   *uuid.UUID  // single-value compat; ignored when AssigneeIDs is non-empty
	AssigneeIDs  []uuid.UUID // multi-value; takes priority over AssigneeID
	AssigneeNull bool        // true → only tasks with no rows in task_assignees
	ParentTaskID *uuid.UUID  // non-nil → only subtasks of this parent
	TaskTypeIDs  []uuid.UUID // multi-value; when non-empty, only tasks of these types
	TaskTypeNull bool        // true → only tasks where task_type_id IS NULL
	BacklogOnly  bool        // true → only tasks where sprint_id IS NULL
	CursorAfter  *string     // opaque base64 cursor; when set, replaces offset-based paging
	// Search, when non-nil and non-blank, restricts results to tasks whose title
	// or "#<task_number>" id contains the text (case-insensitive).
	Search *string
}

// CustomFieldDefinitionRepository defines persistence operations for custom
// field definitions.
type CustomFieldDefinitionRepository interface {
	ListCustomFieldDefinitions(ctx context.Context, projectID uuid.UUID) ([]*CustomFieldDefinition, error)
	FindCustomFieldDefinitionByID(ctx context.Context, id uuid.UUID) (*CustomFieldDefinition, error)
	CreateCustomFieldDefinition(ctx context.Context, f *CustomFieldDefinition) error
	UpdateCustomFieldDefinition(ctx context.Context, f *CustomFieldDefinition) error
	DeleteCustomFieldDefinition(ctx context.Context, id uuid.UUID) error
	// ClearCustomFieldValues strips one field_key from every task's
	// custom_fields JSONB in a project (used on a field type change).
	ClearCustomFieldValues(ctx context.Context, projectID uuid.UUID, fieldKey string) error
}
