package versiondom

import (
	"context"

	"github.com/google/uuid"
)

// Repository is the persistence contract for the version aggregate.
type Repository interface {
	ListVersions(ctx context.Context, projectID uuid.UUID) ([]*Version, error)
	FindVersionByID(ctx context.Context, id uuid.UUID) (*Version, error)
	CreateVersion(ctx context.Context, v *Version) error
	UpdateVersion(ctx context.Context, v *Version) error
	DeleteVersion(ctx context.Context, id uuid.UUID) error
}
