// Package worklogdom defines the task worklog (time-tracking) aggregate and its
// domain contracts. A worklog is a positive amount of time logged against a task
// by a project member.
package worklogdom

import (
	"time"

	"github.com/google/uuid"
)

// Worklog is a time-tracking entry recorded against a task. MemberID is the
// acting project member and may be nil when the actor is not a project member.
type Worklog struct {
	ID        uuid.UUID
	TaskID    uuid.UUID
	MemberID  *uuid.UUID
	Minutes   int
	Note      string
	LoggedAt  time.Time
	CreatedAt time.Time
}
