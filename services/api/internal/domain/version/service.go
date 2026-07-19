package versiondom

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Service defines project version use cases.
type Service interface {
	ListVersions(ctx context.Context, projectID uuid.UUID) ([]*Version, error)
	CreateVersion(ctx context.Context, in CreateVersionInput) (*Version, error)
	// UpdateVersion updates the version identified by id, verifying it belongs to projectID.
	UpdateVersion(ctx context.Context, projectID, id uuid.UUID, in UpdateVersionInput) (*Version, error)
	// DeleteVersion removes the version identified by id, verifying it belongs to projectID.
	DeleteVersion(ctx context.Context, projectID, id uuid.UUID) error
}

// CreateVersionInput carries fields required to create a version.
type CreateVersionInput struct {
	ProjectID   uuid.UUID
	Name        string
	Description string
	Released    bool
	ReleaseDate *time.Time
	Archived    bool
}

// UpdateVersionInput carries mutable version fields for a PATCH operation.
// Name is applied when non-empty. For the pointer fields a nil pointer means the
// field was absent and must not be overwritten; Description and ReleaseDate use
// the double-pointer convention (nil = absent, &nil = clear, &&value = set).
type UpdateVersionInput struct {
	Name        string
	Description **string
	Released    *bool
	ReleaseDate **time.Time
	Archived    *bool
}
