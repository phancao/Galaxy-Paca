// Package worklogsvc implements the task worklog (time-tracking) application service.
package worklogsvc

import (
	"context"
	"errors"
	"strings"
	"time"

	taskdom "github.com/Paca-AI/api/internal/domain/task"
	worklogdom "github.com/Paca-AI/api/internal/domain/worklog"
	"github.com/google/uuid"
)

// Service is the concrete implementation of worklogdom.Service.
type Service struct {
	repo        worklogdom.Repository
	taskChecker worklogdom.TaskOwnerChecker
}

// New returns a configured worklog service. taskChecker scopes worklog
// operations to tasks within the requested project.
func New(repo worklogdom.Repository, taskChecker worklogdom.TaskOwnerChecker) *Service {
	return &Service{repo: repo, taskChecker: taskChecker}
}

// ListWorklogs returns all worklogs for a task, verifying it belongs to projectID.
func (s *Service) ListWorklogs(ctx context.Context, projectID, taskID uuid.UUID) ([]*worklogdom.Worklog, error) {
	if err := s.taskChecker.TaskBelongsToProject(ctx, projectID, taskID); err != nil {
		return nil, err
	}
	return s.repo.ListWorklogs(ctx, taskID)
}

// CreateWorklog logs work against a task, verifying it belongs to projectID.
func (s *Service) CreateWorklog(ctx context.Context, in worklogdom.CreateWorklogInput) (*worklogdom.Worklog, error) {
	if in.Minutes <= 0 {
		return nil, worklogdom.ErrWorklogMinutesInvalid
	}
	if err := s.taskChecker.TaskBelongsToProject(ctx, in.ProjectID, in.TaskID); err != nil {
		return nil, err
	}

	now := time.Now()
	loggedAt := now
	if in.LoggedAt != nil {
		loggedAt = *in.LoggedAt
	}
	w := &worklogdom.Worklog{
		ID:        uuid.New(),
		TaskID:    in.TaskID,
		MemberID:  in.MemberID,
		Minutes:   in.Minutes,
		Note:      strings.TrimSpace(in.Note),
		LoggedAt:  loggedAt,
		CreatedAt: now,
	}
	if err := s.repo.CreateWorklog(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

// DeleteWorklog removes a worklog, verifying it belongs to taskID within projectID.
func (s *Service) DeleteWorklog(ctx context.Context, projectID, taskID, worklogID uuid.UUID) error {
	if err := s.taskChecker.TaskBelongsToProject(ctx, projectID, taskID); err != nil {
		return err
	}
	w, err := s.repo.FindWorklogByID(ctx, worklogID)
	if err != nil {
		return err
	}
	if w.TaskID != taskID {
		return worklogdom.ErrWorklogNotFound
	}
	return s.repo.DeleteWorklog(ctx, worklogID)
}

// taskOwnerChecker adapts a taskdom.TaskRepository to worklogdom.TaskOwnerChecker
// without depending on the task service. It reuses the existing FindTaskByID
// read and never mutates the tasks table.
type taskOwnerChecker struct {
	repo taskdom.TaskRepository
}

// NewTaskOwnerChecker returns a worklogdom.TaskOwnerChecker backed by the task
// repository.
func NewTaskOwnerChecker(repo taskdom.TaskRepository) worklogdom.TaskOwnerChecker {
	return &taskOwnerChecker{repo: repo}
}

func (c *taskOwnerChecker) TaskBelongsToProject(ctx context.Context, projectID, taskID uuid.UUID) error {
	t, err := c.repo.FindTaskByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, taskdom.ErrTaskNotFound) {
			return worklogdom.ErrWorklogTaskNotInProject
		}
		return err
	}
	if t.ProjectID != projectID {
		return worklogdom.ErrWorklogTaskNotInProject
	}
	return nil
}
