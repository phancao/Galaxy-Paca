// Package versiondom defines the project version (fixVersion) aggregate and its
// domain contracts. A version is a named release a project's tasks can target;
// released/archived are lifecycle flags and release_date the planned or actual
// ship date.
package versiondom

import (
	"time"

	"github.com/google/uuid"
)

// Version is a project release / fix-version aggregate.
type Version struct {
	ID          uuid.UUID
	ProjectID   uuid.UUID
	Name        string
	Description string
	Released    bool
	ReleaseDate *time.Time
	Archived    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
