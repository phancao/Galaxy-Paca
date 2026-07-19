// Package wikispacedom models the ADR-042 Paca↔Wiki associations. The Galaxy
// AI Wiki is the system of record for documentation content; Paca stores only
// which Wiki Folder (space) backs a project and which Wiki Records (pages)
// are linked to a task. Wiki ids are foreign identifiers, kept as strings.
package wikispacedom

import (
	"time"

	"github.com/google/uuid"
)

// ProjectWikiSpace maps one project to its Wiki Folder.
type ProjectWikiSpace struct {
	ProjectID    uuid.UUID
	WikiFolderID string
	WikiTeamID   string
	// WikiURL is the browser-facing absolute URL of the folder.
	WikiURL   string
	CreatedBy *uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TaskWikiLink associates a task with one Wiki Record (page).
type TaskWikiLink struct {
	ID           uuid.UUID
	TaskID       uuid.UUID
	WikiRecordID string
	// WikiURL / Title are denormalised for rendering the "Linked pages"
	// panel without a Wiki round-trip.
	WikiURL   string
	Title     string
	CreatedBy *uuid.UUID
	CreatedAt time.Time
}
