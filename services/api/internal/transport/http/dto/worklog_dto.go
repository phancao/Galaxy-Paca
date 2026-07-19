package dto

import (
	"time"

	worklogdom "github.com/Paca-AI/api/internal/domain/worklog"
	"github.com/google/uuid"
)

// CreateWorklogRequest is the body for POST /projects/:projectId/tasks/:taskId/worklogs.
// LoggedAt is optional and defaults to the current time when omitted.
type CreateWorklogRequest struct {
	Minutes  int        `json:"minutes" binding:"required"`
	Note     string     `json:"note"`
	LoggedAt *time.Time `json:"logged_at"`
}

// WorklogResponse is the public representation of a task worklog.
type WorklogResponse struct {
	ID        uuid.UUID  `json:"id"`
	TaskID    uuid.UUID  `json:"task_id"`
	MemberID  *uuid.UUID `json:"member_id,omitempty"`
	Minutes   int        `json:"minutes"`
	Note      string     `json:"note"`
	LoggedAt  time.Time  `json:"logged_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// WorklogFromEntity maps a domain Worklog to a WorklogResponse DTO.
func WorklogFromEntity(w *worklogdom.Worklog) WorklogResponse {
	return WorklogResponse{
		ID:        w.ID,
		TaskID:    w.TaskID,
		MemberID:  w.MemberID,
		Minutes:   w.Minutes,
		Note:      w.Note,
		LoggedAt:  w.LoggedAt,
		CreatedAt: w.CreatedAt,
	}
}
