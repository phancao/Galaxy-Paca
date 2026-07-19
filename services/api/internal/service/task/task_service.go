// Package tasksvc implements task management application services.
package tasksvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	taskdom "github.com/Paca-AI/api/internal/domain/task"
	"github.com/google/uuid"
)

var reservedSystemTypeNames = map[string]bool{
	"Epic": true,
}

// workflowStatusChecker is the minimal workflow-domain surface the task
// service needs to refuse deleting a status that automation still depends on.
type workflowStatusChecker interface {
	StatusUsedByWorkflow(ctx context.Context, statusID uuid.UUID) (bool, error)
}

// Service is the concrete implementation of taskdom.Service.
type Service struct {
	repo            taskdom.Repository
	workflowChecker workflowStatusChecker
}

// New returns a configured task service.
func New(repo taskdom.Repository) *Service {
	return &Service{repo: repo}
}

// WithWorkflowStatusChecker configures a check that refuses to delete a task
// status still referenced by an automation workflow's rules or transitions.
// Without it, DeleteTaskStatus does not guard against this (e.g. in tests).
func (s *Service) WithWorkflowStatusChecker(checker workflowStatusChecker) *Service {
	s.workflowChecker = checker
	return s
}

// --- Task Types -------------------------------------------------------------

// ListTaskTypes returns all task types for a project.
func (s *Service) ListTaskTypes(ctx context.Context, projectID uuid.UUID) ([]*taskdom.TaskType, error) {
	return s.repo.ListTaskTypes(ctx, projectID)
}

// GetTaskType returns the task type with the given ID.
func (s *Service) GetTaskType(ctx context.Context, id uuid.UUID) (*taskdom.TaskType, error) {
	return s.repo.FindTaskTypeByID(ctx, id)
}

// CreateTaskType creates a new task type for the given project.
func (s *Service) CreateTaskType(ctx context.Context, in taskdom.CreateTaskTypeInput) (*taskdom.TaskType, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, taskdom.ErrTypeNameInvalid
	}
	if reservedSystemTypeNames[name] {
		return nil, taskdom.ErrTypeNameReserved
	}

	now := time.Now()
	t := &taskdom.TaskType{
		ID:          uuid.New(),
		ProjectID:   in.ProjectID,
		Name:        name,
		Icon:        in.Icon,
		Color:       in.Color,
		Description: in.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.CreateTaskType(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateTaskType updates the mutable fields of an existing task type.
func (s *Service) UpdateTaskType(ctx context.Context, projectID, id uuid.UUID, in taskdom.UpdateTaskTypeInput) (*taskdom.TaskType, error) {
	t, err := s.repo.FindTaskTypeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.ProjectID != projectID {
		return nil, taskdom.ErrTypeNotFound
	}
	if t.IsSystem {
		return nil, taskdom.ErrTypeIsSystem
	}

	if name := strings.TrimSpace(in.Name); name != "" {
		t.Name = name
	}
	if in.Icon != nil {
		t.Icon = *in.Icon
	}
	if in.Color != nil {
		t.Color = *in.Color
	}
	if in.Description != nil {
		t.Description = *in.Description
	}
	t.UpdatedAt = time.Now()

	if err := s.repo.UpdateTaskType(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// DeleteTaskType removes a task type by ID.
func (s *Service) DeleteTaskType(ctx context.Context, projectID, id uuid.UUID) error {
	t, err := s.repo.FindTaskTypeByID(ctx, id)
	if err != nil {
		return err
	}
	if t.ProjectID != projectID {
		return taskdom.ErrTypeNotFound
	}
	if t.IsSystem {
		return taskdom.ErrTypeIsSystem
	}
	return s.repo.DeleteTaskType(ctx, id)
}

// SetDefaultTaskType marks typeID as the project's default task type,
// clearing the flag on all other types in the same project.
func (s *Service) SetDefaultTaskType(ctx context.Context, projectID, typeID uuid.UUID) (*taskdom.TaskType, error) {
	if err := s.repo.SetDefaultTaskType(ctx, projectID, typeID); err != nil {
		return nil, err
	}
	return s.repo.FindTaskTypeByID(ctx, typeID)
}

// --- Task Statuses ----------------------------------------------------------

// ListTaskStatuses returns all task statuses for a project.
func (s *Service) ListTaskStatuses(ctx context.Context, projectID uuid.UUID) ([]*taskdom.TaskStatus, error) {
	return s.repo.ListTaskStatuses(ctx, projectID)
}

// GetTaskStatus returns the task status with the given ID.
func (s *Service) GetTaskStatus(ctx context.Context, id uuid.UUID) (*taskdom.TaskStatus, error) {
	return s.repo.FindTaskStatusByID(ctx, id)
}

// CreateTaskStatus creates a new task status for the given project.
func (s *Service) CreateTaskStatus(ctx context.Context, in taskdom.CreateTaskStatusInput) (*taskdom.TaskStatus, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, taskdom.ErrStatusNameInvalid
	}
	if !taskdom.ValidStatusCategories[in.Category] {
		return nil, taskdom.ErrStatusCategoryInvalid
	}

	now := time.Now()
	st := &taskdom.TaskStatus{
		ID:        uuid.New(),
		ProjectID: in.ProjectID,
		Name:      name,
		Color:     in.Color,
		Position:  in.Position,
		Category:  in.Category,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.CreateTaskStatus(ctx, st); err != nil {
		return nil, err
	}
	return st, nil
}

// UpdateTaskStatus updates the mutable fields of an existing task status.
func (s *Service) UpdateTaskStatus(ctx context.Context, projectID, id uuid.UUID, in taskdom.UpdateTaskStatusInput) (*taskdom.TaskStatus, error) {
	st, err := s.repo.FindTaskStatusByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if st.ProjectID != projectID {
		return nil, taskdom.ErrStatusNotFound
	}

	if name := strings.TrimSpace(in.Name); name != "" {
		st.Name = name
	}
	if in.Color != nil {
		st.Color = in.Color
	}
	if in.Position != nil {
		st.Position = *in.Position
	}
	if in.Category != nil {
		if !taskdom.ValidStatusCategories[*in.Category] {
			return nil, taskdom.ErrStatusCategoryInvalid
		}
		st.Category = *in.Category
	}
	st.UpdatedAt = time.Now()

	if err := s.repo.UpdateTaskStatus(ctx, st); err != nil {
		return nil, err
	}
	return st, nil
}

// DeleteTaskStatus removes a task status by ID.
func (s *Service) DeleteTaskStatus(ctx context.Context, projectID, id uuid.UUID) error {
	st, err := s.repo.FindTaskStatusByID(ctx, id)
	if err != nil {
		return err
	}
	if st.ProjectID != projectID {
		return taskdom.ErrStatusNotFound
	}
	if s.workflowChecker != nil {
		used, err := s.workflowChecker.StatusUsedByWorkflow(ctx, id)
		if err != nil {
			return err
		}
		if used {
			return taskdom.ErrStatusInUseByWorkflow
		}
	}
	return s.repo.DeleteTaskStatus(ctx, id)
}

// SetDefaultTaskStatus marks statusID as the project's default task status,
// returning the updated status.
func (s *Service) SetDefaultTaskStatus(ctx context.Context, projectID, statusID uuid.UUID) (*taskdom.TaskStatus, error) {
	if err := s.repo.SetDefaultTaskStatus(ctx, projectID, statusID); err != nil {
		return nil, err
	}
	return s.repo.FindTaskStatusByID(ctx, statusID)
}

// ReorderTaskStatuses persists a new display order for the project's task
// statuses, assigning position = index in statusIDs to each status.
func (s *Service) ReorderTaskStatuses(ctx context.Context, projectID uuid.UUID, statusIDs []uuid.UUID) error {
	if len(statusIDs) == 0 {
		return taskdom.ErrStatusReorderInvalid
	}
	return s.repo.ReorderTaskStatuses(ctx, projectID, statusIDs)
}

// isEpicTaskType returns whether typeID belongs to the system Epic type.
func (s *Service) isEpicTaskType(ctx context.Context, typeID *uuid.UUID) (bool, error) {
	if typeID == nil {
		return false, nil
	}
	t, err := s.repo.FindTaskTypeByID(ctx, *typeID)
	if err != nil {
		return false, err
	}
	return t.IsSystem && t.Name == "Epic", nil
}

// wouldCreateCycle reports whether making proposedParentID the parent of taskID
// would introduce a directed cycle in the task hierarchy.
func (s *Service) wouldCreateCycle(ctx context.Context, taskID, proposedParentID uuid.UUID) bool {
	current := proposedParentID
	const maxDepth = 50
	for range maxDepth {
		if current == taskID {
			return true
		}
		t, err := s.repo.FindTaskByID(ctx, current)
		if err != nil || t.ParentTaskID == nil {
			return false
		}
		current = *t.ParentTaskID
	}
	return false
}

// --- Tasks ------------------------------------------------------------------

// ListTasks returns a page of tasks. When filter.CursorAfter is nil, returns from
// the beginning. When set, returns tasks after the cursor position.
// Returns hasMore=true when a next page exists.
func (s *Service) ListTasks(ctx context.Context, projectID uuid.UUID, filter taskdom.TaskFilter, pageSize int, sort taskdom.TaskSort) ([]*taskdom.Task, bool, error) {
	if pageSize < 1 {
		pageSize = 20
	}
	return s.repo.ListTasks(ctx, projectID, filter, pageSize, sort)
}

// CountTasks returns the number of tasks in a project matching the given filter.
func (s *Service) CountTasks(ctx context.Context, projectID uuid.UUID, filter taskdom.TaskFilter) (int64, error) {
	return s.repo.CountTasks(ctx, projectID, filter)
}

// SumTaskField sums a numeric field across all matching tasks, ignoring pagination.
func (s *Service) SumTaskField(ctx context.Context, projectID uuid.UUID, filter taskdom.TaskFilter, fieldKey string) (float64, error) {
	return s.repo.SumTaskField(ctx, projectID, filter, fieldKey)
}

// GetTask returns the task with the given ID, verifying it belongs to projectID.
func (s *Service) GetTask(ctx context.Context, projectID, id uuid.UUID) (*taskdom.Task, error) {
	t, err := s.repo.FindTaskByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.ProjectID != projectID {
		return nil, taskdom.ErrTaskNotFound
	}
	return t, nil
}

// GetTaskByNumber returns the task with the given project-scoped sequential number.
func (s *Service) GetTaskByNumber(ctx context.Context, projectID uuid.UUID, taskNumber int64) (*taskdom.Task, error) {
	return s.repo.FindTaskByNumber(ctx, projectID, taskNumber)
}

// CreateTask creates a new task. When TaskTypeID or StatusID are not provided,
// the project's default task type / status is resolved automatically.
func (s *Service) CreateTask(ctx context.Context, in taskdom.CreateTaskInput) (*taskdom.Task, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, taskdom.ErrTaskTitleInvalid
	}

	if in.ParentTaskID != nil {
		isEpic, err := s.isEpicTaskType(ctx, in.TaskTypeID)
		if err != nil {
			return nil, err
		}
		if isEpic {
			return nil, taskdom.ErrEpicCannotHaveParent
		}
	}

	taskTypeID := in.TaskTypeID
	if taskTypeID == nil {
		if dt, err := s.repo.FindDefaultTaskType(ctx, in.ProjectID); err != nil {
			return nil, err
		} else if dt != nil {
			taskTypeID = &dt.ID
		}
	}

	statusID := in.StatusID
	if statusID == nil {
		if ds, err := s.repo.FindDefaultTaskStatus(ctx, in.ProjectID); err != nil {
			return nil, err
		} else if ds != nil {
			statusID = &ds.ID
		}
	}

	cf := in.CustomFields
	if cf == nil {
		cf = map[string]any{}
	}
	// Validate/normalize custom fields against the definitions applicable to
	// this task's type: reject bad types/options, fill defaults, enforce
	// required at creation.
	cfDefs, err := s.repo.ListCustomFieldDefinitions(ctx, in.ProjectID)
	if err != nil {
		return nil, err
	}
	cfDefs = taskdom.ApplicableCustomFields(cfDefs, taskTypeID)
	cf, err = taskdom.ValidateCustomFields(cfDefs, cf, true)
	if err != nil {
		return nil, err
	}
	tags := in.Tags
	if tags == nil {
		tags = []string{}
	}
	assigneeIDs := in.AssigneeIDs
	if assigneeIDs == nil {
		assigneeIDs = []uuid.UUID{}
	}

	now := time.Now()
	t := &taskdom.Task{
		ID:              uuid.New(),
		ProjectID:       in.ProjectID,
		TaskTypeID:      taskTypeID,
		StatusID:        statusID,
		SprintID:        in.SprintID,
		ParentTaskID:    in.ParentTaskID,
		Title:           title,
		Description:     in.Description,
		Importance:      in.Importance,
		StoryPoints:     in.StoryPoints,
		AssigneeIDs:     assigneeIDs,
		ReporterID:      in.ReporterID,
		CustomFields:    cf,
		StartDate:       in.StartDate,
		DueDate:         in.DueDate,
		Tags:            tags,
		EstimateMinutes: in.EstimateMinutes,
		VersionID:       in.VersionID,
		ComponentID:     in.ComponentID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.repo.CreateTask(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateTask updates the mutable fields of an existing task.
func (s *Service) UpdateTask(ctx context.Context, projectID, id uuid.UUID, in taskdom.UpdateTaskInput) (*taskdom.Task, error) {
	t, err := s.repo.FindTaskByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.ProjectID != projectID {
		return nil, taskdom.ErrTaskNotFound
	}
	oldStatusID := t.StatusID

	if title := strings.TrimSpace(in.Title); title != "" {
		t.Title = title
	}

	// Compute the effective parent and type IDs after the update to validate constraints.
	effectiveParentID := t.ParentTaskID
	if in.ParentTaskID != nil {
		effectiveParentID = *in.ParentTaskID
	}
	effectiveTypeID := t.TaskTypeID
	if in.TaskTypeID != nil {
		effectiveTypeID = *in.TaskTypeID
	}
	// Validate parent constraints using the post-update effective values.
	if effectiveParentID != nil {
		if *effectiveParentID == t.ID {
			return nil, taskdom.ErrTaskCannotBeOwnParent
		}
		if s.wouldCreateCycle(ctx, t.ID, *effectiveParentID) {
			return nil, taskdom.ErrTaskParentCycleDetected
		}
		isEpic, err := s.isEpicTaskType(ctx, effectiveTypeID)
		if err != nil {
			return nil, err
		}
		if isEpic {
			return nil, taskdom.ErrEpicCannotHaveParent
		}
	}

	if in.TaskTypeID != nil {
		t.TaskTypeID = *in.TaskTypeID
	}
	if in.StatusID != nil {
		t.StatusID = *in.StatusID
	}
	if in.SprintID != nil {
		t.SprintID = *in.SprintID
	}
	if in.ParentTaskID != nil {
		t.ParentTaskID = *in.ParentTaskID
	}
	if in.Description != nil {
		t.Description = *in.Description
	}
	if in.Importance != nil {
		t.Importance = *in.Importance
	}
	if in.StoryPoints != nil {
		t.StoryPoints = *in.StoryPoints
	}
	if in.AssigneeIDs != nil {
		t.AssigneeIDs = *in.AssigneeIDs
	}
	if in.ReporterID != nil {
		t.ReporterID = *in.ReporterID
	}
	if in.CustomFields != nil {
		cfDefs, err := s.repo.ListCustomFieldDefinitions(ctx, t.ProjectID)
		if err != nil {
			return nil, err
		}
		cfDefs = taskdom.ApplicableCustomFields(cfDefs, t.TaskTypeID)
		// enforceAllRequired=false: don't block edits of tasks that predate a
		// required field; still reject bad values or an explicit clear of a
		// required field.
		validated, err := taskdom.ValidateCustomFields(cfDefs, *in.CustomFields, false)
		if err != nil {
			return nil, err
		}
		t.CustomFields = validated
	}
	if in.StartDate != nil {
		t.StartDate = *in.StartDate
	}
	if in.DueDate != nil {
		t.DueDate = *in.DueDate
	}
	if in.Tags != nil {
		t.Tags = *in.Tags
	}
	if in.EstimateMinutes != nil {
		t.EstimateMinutes = *in.EstimateMinutes
	}
	if in.VersionID != nil {
		t.VersionID = *in.VersionID
	}
	if in.ComponentID != nil {
		t.ComponentID = *in.ComponentID
	}

	// Enforce the project's workflow (opt-in) on any status change, using the
	// task's resulting custom fields for required-field checks.
	if err := s.enforceStatusTransition(ctx, t.ProjectID, oldStatusID, t.StatusID, t.TaskTypeID, t.CustomFields); err != nil {
		return nil, err
	}

	t.UpdatedAt = time.Now()

	if err := s.repo.UpdateTask(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// DeleteTask soft-deletes a task by ID, verifying it belongs to projectID.
func (s *Service) DeleteTask(ctx context.Context, projectID, id uuid.UUID) error {
	t, err := s.repo.FindTaskByID(ctx, id)
	if err != nil {
		return err
	}
	if t.ProjectID != projectID {
		return taskdom.ErrTaskNotFound
	}
	return s.repo.DeleteTask(ctx, id)
}

// --- Custom Field Definitions -----------------------------------------------

// ListCustomFieldDefinitions returns all custom field definitions for a project.
func (s *Service) ListCustomFieldDefinitions(ctx context.Context, projectID uuid.UUID) ([]*taskdom.CustomFieldDefinition, error) {
	return s.repo.ListCustomFieldDefinitions(ctx, projectID)
}

// GetCustomFieldDefinition returns the custom field definition with the given ID,
// verifying it belongs to projectID.
func (s *Service) GetCustomFieldDefinition(ctx context.Context, projectID, id uuid.UUID) (*taskdom.CustomFieldDefinition, error) {
	f, err := s.repo.FindCustomFieldDefinitionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.ProjectID != projectID {
		return nil, taskdom.ErrCustomFieldNotFound
	}
	return f, nil
}

// CreateCustomFieldDefinition creates a new custom field definition.
func (s *Service) CreateCustomFieldDefinition(ctx context.Context, in taskdom.CreateCustomFieldDefinitionInput) (*taskdom.CustomFieldDefinition, error) {
	fieldKey := strings.TrimSpace(in.FieldKey)
	if fieldKey == "" {
		return nil, taskdom.ErrCustomFieldKeyInvalid
	}
	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		return nil, taskdom.ErrCustomFieldNameInvalid
	}
	if !taskdom.ValidFieldTypes[in.FieldType] {
		return nil, taskdom.ErrCustomFieldTypeInvalid
	}

	opts := in.Options
	if opts == nil {
		opts = []string{}
	}
	casc := in.CascadeOptions
	if casc == nil {
		casc = []taskdom.CascadeOption{}
	}
	if err := taskdom.ValidateFieldDefinition(in.FieldType, opts, casc); err != nil {
		return nil, err
	}
	defaultVal, err := taskdom.CoerceDefaultValue(in.FieldType, opts, casc, in.DefaultValue)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	f := &taskdom.CustomFieldDefinition{
		ID:             uuid.New(),
		ProjectID:      in.ProjectID,
		FieldKey:       fieldKey,
		DisplayName:    displayName,
		FieldType:      in.FieldType,
		Options:        opts,
		CascadeOptions: casc,
		IsRequired:     in.IsRequired,
		DefaultValue:   defaultVal,
		TaskTypeID:     in.TaskTypeID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repo.CreateCustomFieldDefinition(ctx, f); err != nil {
		return nil, err
	}
	return f, nil
}

// UpdateCustomFieldDefinition updates the mutable fields of a custom field
// definition. The field_key is immutable after creation.
func (s *Service) UpdateCustomFieldDefinition(ctx context.Context, projectID, id uuid.UUID, in taskdom.UpdateCustomFieldDefinitionInput) (*taskdom.CustomFieldDefinition, error) {
	f, err := s.repo.FindCustomFieldDefinitionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.ProjectID != projectID {
		return nil, taskdom.ErrCustomFieldNotFound
	}
	oldType := f.FieldType

	if displayName := strings.TrimSpace(in.DisplayName); displayName != "" {
		f.DisplayName = displayName
	}
	if in.FieldType != nil {
		if !taskdom.ValidFieldTypes[*in.FieldType] {
			return nil, taskdom.ErrCustomFieldTypeInvalid
		}
		f.FieldType = *in.FieldType
	}
	if in.Options != nil {
		f.Options = in.Options
	}
	if in.CascadeOptions != nil {
		f.CascadeOptions = in.CascadeOptions
	}
	if in.IsRequired != nil {
		f.IsRequired = *in.IsRequired
	}
	// Guardrail: the effective type + options must remain consistent (a
	// select/multi_select still needs an option; cascading_select a parent).
	if err := taskdom.ValidateFieldDefinition(f.FieldType, f.Options, f.CascadeOptions); err != nil {
		return nil, err
	}
	if in.DefaultValue != nil {
		defaultVal, err := taskdom.CoerceDefaultValue(f.FieldType, f.Options, f.CascadeOptions, *in.DefaultValue)
		if err != nil {
			return nil, err
		}
		f.DefaultValue = defaultVal
	} else if f.DefaultValue != nil {
		// Re-coerce the existing default against a possibly-changed type/options
		// so a type change never leaves an invalid stored default.
		defaultVal, err := taskdom.CoerceDefaultValue(f.FieldType, f.Options, f.CascadeOptions, f.DefaultValue)
		if err != nil {
			f.DefaultValue = nil
		} else {
			f.DefaultValue = defaultVal
		}
	}
	f.UpdatedAt = time.Now()

	if err := s.repo.UpdateCustomFieldDefinition(ctx, f); err != nil {
		return nil, err
	}
	// A type change makes previously-stored values type-incompatible; strip
	// them so tasks never carry a value that no longer validates.
	if oldType != f.FieldType {
		if err := s.repo.ClearCustomFieldValues(ctx, f.ProjectID, f.FieldKey); err != nil {
			return nil, err
		}
	}
	return f, nil
}

// DeleteCustomFieldDefinition removes a custom field definition by ID,
// verifying it belongs to projectID.
func (s *Service) DeleteCustomFieldDefinition(ctx context.Context, projectID, id uuid.UUID) error {
	f, err := s.repo.FindCustomFieldDefinitionByID(ctx, id)
	if err != nil {
		return err
	}
	if f.ProjectID != projectID {
		return taskdom.ErrCustomFieldNotFound
	}
	return s.repo.DeleteCustomFieldDefinition(ctx, id)
}

// CopyConfiguration copies a source project's task schema — task types,
// statuses, custom fields, and workflow transitions — into a target project so
// a "template" project can be reused instead of re-seeding every project by
// hand (ADR-040 Phase 3, additive: it never mutates existing rows). Items that
// already exist in the target are skipped (types/statuses by name, custom
// fields by field_key), and transitions/field scopes are remapped from source
// ids to the matching target ids by name.
func (s *Service) CopyConfiguration(ctx context.Context, sourceProjectID, targetProjectID uuid.UUID) error {
	if sourceProjectID == targetProjectID {
		return nil
	}
	now := time.Now()

	// --- Task types (skip system + existing-by-name) ---
	srcTypes, err := s.repo.ListTaskTypes(ctx, sourceProjectID)
	if err != nil {
		return err
	}
	tgtTypes, err := s.repo.ListTaskTypes(ctx, targetProjectID)
	if err != nil {
		return err
	}
	typeByName := make(map[string]uuid.UUID, len(tgtTypes))
	for _, t := range tgtTypes {
		typeByName[t.Name] = t.ID
	}
	for _, st := range srcTypes {
		if st.IsSystem {
			continue
		}
		if _, exists := typeByName[st.Name]; exists {
			continue
		}
		nt := &taskdom.TaskType{
			ID: uuid.New(), ProjectID: targetProjectID, Name: st.Name,
			Icon: st.Icon, Color: st.Color, Description: st.Description,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := s.repo.CreateTaskType(ctx, nt); err != nil {
			return err
		}
		typeByName[st.Name] = nt.ID
	}

	// --- Statuses (skip existing-by-name) ---
	srcStatuses, err := s.repo.ListTaskStatuses(ctx, sourceProjectID)
	if err != nil {
		return err
	}
	tgtStatuses, err := s.repo.ListTaskStatuses(ctx, targetProjectID)
	if err != nil {
		return err
	}
	statusByName := make(map[string]uuid.UUID, len(tgtStatuses))
	for _, st := range tgtStatuses {
		statusByName[st.Name] = st.ID
	}
	for _, ss := range srcStatuses {
		if _, exists := statusByName[ss.Name]; exists {
			continue
		}
		ns := &taskdom.TaskStatus{
			ID: uuid.New(), ProjectID: targetProjectID, Name: ss.Name,
			Color: ss.Color, Position: ss.Position, Category: ss.Category,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := s.repo.CreateTaskStatus(ctx, ns); err != nil {
			return err
		}
		statusByName[ss.Name] = ns.ID
	}

	// --- Custom fields (skip existing-by-key; remap type scope by name) ---
	srcFields, err := s.repo.ListCustomFieldDefinitions(ctx, sourceProjectID)
	if err != nil {
		return err
	}
	tgtFields, err := s.repo.ListCustomFieldDefinitions(ctx, targetProjectID)
	if err != nil {
		return err
	}
	fieldKeys := make(map[string]bool, len(tgtFields))
	for _, f := range tgtFields {
		fieldKeys[f.FieldKey] = true
	}
	for _, sf := range srcFields {
		if fieldKeys[sf.FieldKey] {
			continue
		}
		var typeScope *uuid.UUID
		if sf.TaskTypeID != nil {
			// Map the source type scope to the target type of the same name;
			// drop the scope if that type doesn't exist in the target.
			for _, st := range srcTypes {
				if st.ID == *sf.TaskTypeID {
					if id, ok := typeByName[st.Name]; ok {
						idCopy := id
						typeScope = &idCopy
					}
					break
				}
			}
		}
		nf := &taskdom.CustomFieldDefinition{
			ID: uuid.New(), ProjectID: targetProjectID, FieldKey: sf.FieldKey,
			DisplayName: sf.DisplayName, FieldType: sf.FieldType, Options: sf.Options,
			CascadeOptions: sf.CascadeOptions,
			IsRequired:     sf.IsRequired, DefaultValue: sf.DefaultValue, TaskTypeID: typeScope,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := s.repo.CreateCustomFieldDefinition(ctx, nf); err != nil {
			return err
		}
		fieldKeys[sf.FieldKey] = true
	}

	// --- Workflow transitions (remap status/type ids by name) ---
	srcTransitions, err := s.repo.ListStatusTransitions(ctx, sourceProjectID)
	if err != nil {
		return err
	}
	// Source id -> name lookups for remapping.
	srcStatusName := make(map[uuid.UUID]string, len(srcStatuses))
	for _, st := range srcStatuses {
		srcStatusName[st.ID] = st.Name
	}
	srcTypeName := make(map[uuid.UUID]string, len(srcTypes))
	for _, st := range srcTypes {
		srcTypeName[st.ID] = st.Name
	}
	for _, tr := range srcTransitions {
		toID, ok := statusByName[srcStatusName[tr.ToStatusID]]
		if !ok {
			continue // destination status not present in target
		}
		var fromID *uuid.UUID
		if tr.FromStatusID != nil {
			id, ok := statusByName[srcStatusName[*tr.FromStatusID]]
			if !ok {
				continue
			}
			fromID = &id
		}
		var typeID *uuid.UUID
		if tr.TaskTypeID != nil {
			id, ok := typeByName[srcTypeName[*tr.TaskTypeID]]
			if !ok {
				continue
			}
			typeID = &id
		}
		nt := &taskdom.StatusTransition{
			ID: uuid.New(), ProjectID: targetProjectID, TaskTypeID: typeID,
			FromStatusID: fromID, ToStatusID: toID, RequiredFields: tr.RequiredFields,
			CreatedAt: now,
		}
		// Duplicate rules (same project/type/from/to) are refused by the unique
		// index; ignore that so re-copying is idempotent.
		if err := s.repo.CreateStatusTransition(ctx, nt); err != nil && !errors.Is(err, taskdom.ErrTransitionInvalid) {
			return err
		}
	}

	return nil
}

// --- Status Transitions (workflow engine, ADR-040) --------------------------

// ListStatusTransitions returns all workflow transition rules for a project.
func (s *Service) ListStatusTransitions(ctx context.Context, projectID uuid.UUID) ([]*taskdom.StatusTransition, error) {
	return s.repo.ListStatusTransitions(ctx, projectID)
}

// CreateStatusTransition declares one allowed workflow transition.
func (s *Service) CreateStatusTransition(ctx context.Context, in taskdom.CreateStatusTransitionInput) (*taskdom.StatusTransition, error) {
	if in.ToStatusID == uuid.Nil {
		return nil, taskdom.ErrTransitionInvalid
	}
	// A same-from/to rule is meaningless (staying put is always allowed).
	if in.FromStatusID != nil && *in.FromStatusID == in.ToStatusID {
		return nil, taskdom.ErrTransitionInvalid
	}
	rf := in.RequiredFields
	if rf == nil {
		rf = []string{}
	}
	t := &taskdom.StatusTransition{
		ID:             uuid.New(),
		ProjectID:      in.ProjectID,
		TaskTypeID:     in.TaskTypeID,
		FromStatusID:   in.FromStatusID,
		ToStatusID:     in.ToStatusID,
		RequiredFields: rf,
		CreatedAt:      time.Now(),
	}
	if err := s.repo.CreateStatusTransition(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// DeleteStatusTransition removes a transition rule, verifying project ownership.
func (s *Service) DeleteStatusTransition(ctx context.Context, projectID, id uuid.UUID) error {
	return s.repo.DeleteStatusTransition(ctx, projectID, id)
}

// enforceStatusTransition checks that moving a task from oldStatus to newStatus
// is permitted by the project's workflow. No-op when the project has no
// transition rules (free movement), the status is unchanged, or the task had no
// prior status (initial assignment). When a matching rule lists required
// fields, each must be non-empty on the task.
func (s *Service) enforceStatusTransition(ctx context.Context, projectID uuid.UUID, oldStatus, newStatus, typeID *uuid.UUID, cf map[string]any) error {
	if newStatus == nil {
		return nil
	}
	if oldStatus != nil && *oldStatus == *newStatus {
		return nil
	}
	transitions, err := s.repo.ListStatusTransitions(ctx, projectID)
	if err != nil {
		return err
	}
	if len(transitions) == 0 {
		return nil // no workflow configured → free movement (backward compatible)
	}
	if oldStatus == nil {
		return nil // initial status assignment is always allowed
	}
	var matched *taskdom.StatusTransition
	for _, tr := range transitions {
		if tr.ToStatusID != *newStatus {
			continue
		}
		if tr.FromStatusID != nil && *tr.FromStatusID != *oldStatus {
			continue
		}
		if tr.TaskTypeID != nil && (typeID == nil || *tr.TaskTypeID != *typeID) {
			continue
		}
		matched = tr
		break
	}
	if matched == nil {
		return taskdom.ErrTransitionNotAllowed
	}
	if key, missing := taskdom.MissingRequiredField(cf, matched.RequiredFields); missing {
		return fmt.Errorf("%w: %s", taskdom.ErrTransitionRequiredField, key)
	}
	return nil
}
