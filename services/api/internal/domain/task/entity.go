// Package taskdom defines the task aggregate and its domain contracts.
package taskdom

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// StatusCategory describes which phase of the workflow a TaskStatus belongs to.
type StatusCategory string

// StatusCategory constants for task workflow phases.
const (
	StatusCategoryBacklog    StatusCategory = "backlog"
	StatusCategoryRefinement StatusCategory = "refinement"
	StatusCategoryReady      StatusCategory = "ready"
	StatusCategoryTodo       StatusCategory = "todo"
	StatusCategoryInProgress StatusCategory = "inprogress"
	StatusCategoryDone       StatusCategory = "done"
)

// ValidStatusCategories is the set of allowed status category values.
var ValidStatusCategories = map[StatusCategory]bool{
	StatusCategoryBacklog:    true,
	StatusCategoryRefinement: true,
	StatusCategoryReady:      true,
	StatusCategoryTodo:       true,
	StatusCategoryInProgress: true,
	StatusCategoryDone:       true,
}

// TaskType categorises tasks within a project.
type TaskType struct {
	ID          uuid.UUID
	ProjectID   uuid.UUID
	Name        string
	Icon        *string
	Color       *string
	Description *string
	IsDefault   bool
	IsSystem    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TaskStatus represents a column in the project's status workflow.
type TaskStatus struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	Name      string
	Color     *string
	Position  int
	Category  StatusCategory
	IsDefault bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// FieldType describes the kind of value a CustomFieldDefinition accepts.
type FieldType string

// FieldType constants.
const (
	FieldTypeText        FieldType = "text"
	FieldTypeNumber      FieldType = "number"
	FieldTypeDate        FieldType = "date"
	FieldTypeSelect      FieldType = "select"
	FieldTypeMultiSelect FieldType = "multi_select"
	FieldTypeBoolean     FieldType = "boolean"
	FieldTypeURL         FieldType = "url"
	// FieldTypeUser stores a project member's UUID (a people picker).
	FieldTypeUser FieldType = "user"
	// FieldTypeLabel stores a free-form array of strings (open vocabulary tags,
	// unlike multi_select which is constrained to predefined options).
	FieldTypeLabel FieldType = "label"
)

// ValidFieldTypes is the set of allowed field type values.
var ValidFieldTypes = map[FieldType]bool{
	FieldTypeText:        true,
	FieldTypeNumber:      true,
	FieldTypeDate:        true,
	FieldTypeSelect:      true,
	FieldTypeMultiSelect: true,
	FieldTypeBoolean:     true,
	FieldTypeURL:         true,
	FieldTypeUser:        true,
	FieldTypeLabel:       true,
}

// CustomFieldDefinition is a project-level schema entry that describes one
// extra field that can be stored in Task.CustomFields under FieldKey.
type CustomFieldDefinition struct {
	ID          uuid.UUID
	ProjectID   uuid.UUID
	FieldKey    string
	DisplayName string
	FieldType   FieldType
	Options     []string // populated for select / multi_select types
	IsRequired  bool
	// DefaultValue is applied to a task's CustomFields when the field is
	// otherwise absent (nil = no default). It is stored as JSONB and is already
	// coerced to the field's type (string / float64 / bool / []string).
	DefaultValue any
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// StatusTransition is one allowed workflow move for a project (ADR-040). A nil
// TaskTypeID applies to every task type; a nil FromStatusID allows the move
// from any source status. RequiredFields lists custom-field field_keys that
// must be non-empty on the task for the transition to be permitted.
type StatusTransition struct {
	ID             uuid.UUID
	ProjectID      uuid.UUID
	TaskTypeID     *uuid.UUID
	FromStatusID   *uuid.UUID
	ToStatusID     uuid.UUID
	RequiredFields []string
	CreatedAt      time.Time
}

// LinkType describes the directional relationship stored in a TaskLink row.
// Inverse display types (is_blocked_by, is_duplicated_by) are computed at
// the service layer and never persisted.
type LinkType string

const (
	// LinkTypeBlocks indicates the source task blocks completion of the target.
	LinkTypeBlocks LinkType = "blocks"
	// LinkTypeRelatesTo indicates the source and target tasks are related.
	LinkTypeRelatesTo LinkType = "relates_to"
	// LinkTypeDuplicates indicates the source task duplicates the target.
	LinkTypeDuplicates LinkType = "duplicates"
)

// ValidLinkTypes is the set of persisted link type values.
var ValidLinkTypes = map[LinkType]bool{
	LinkTypeBlocks:     true,
	LinkTypeRelatesTo:  true,
	LinkTypeDuplicates: true,
}

// TaskLink represents a directed relationship between two tasks.
type TaskLink struct {
	ID           uuid.UUID
	SourceTaskID uuid.UUID
	TargetTaskID uuid.UUID
	LinkType     LinkType
	CreatedBy    *uuid.UUID
	CreatedAt    time.Time
	// LinkedTask is populated on read with a summary of the other task.
	LinkedTask *LinkedTaskSummary
	// DisplayLinkType is the relationship label from the requesting task's
	// perspective (e.g. "is_blocked_by" when this task is the target of a
	// "blocks" link). It is never persisted.
	DisplayLinkType string
}

// LinkedTaskSummary is a lightweight projection of a Task embedded in
// TaskLink responses to avoid N+1 lookups.
type LinkedTaskSummary struct {
	ID         uuid.UUID
	TaskNumber int64
	Title      string
	StatusID   *uuid.UUID
	TaskTypeID *uuid.UUID
}

// Task is the core work item aggregate.
type Task struct {
	ID           uuid.UUID
	ProjectID    uuid.UUID
	TaskNumber   int64
	TaskTypeID   *uuid.UUID
	StatusID     *uuid.UUID
	SprintID     *uuid.UUID
	ParentTaskID *uuid.UUID
	Title        string
	Description  json.RawMessage
	Importance   int
	StoryPoints  *int
	AssigneeIDs  []uuid.UUID
	ReporterID   *uuid.UUID
	CustomFields map[string]any
	StartDate    *time.Time
	DueDate      *time.Time
	Tags         []string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
	// ViewPosition is a transient field populated only when ListTasks is called
	// with a view_position sort (i.e. view_id provided + manual sort). It is not
	// persisted in the tasks table.
	ViewPosition *float64
}
