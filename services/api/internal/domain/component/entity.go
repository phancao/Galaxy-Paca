// Package componentdom defines the project component aggregate and its domain
// contracts. A component is a named functional area of a project, optionally
// owned by a lead project member.
package componentdom

import (
	"time"

	"github.com/google/uuid"
)

// Component is a project component (functional area) aggregate.
type Component struct {
	ID           uuid.UUID
	ProjectID    uuid.UUID
	Name         string
	Description  string
	LeadMemberID *uuid.UUID
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
