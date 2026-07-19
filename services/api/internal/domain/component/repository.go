package componentdom

import (
	"context"

	"github.com/google/uuid"
)

// Repository is the persistence contract for the component aggregate.
type Repository interface {
	ListComponents(ctx context.Context, projectID uuid.UUID) ([]*Component, error)
	FindComponentByID(ctx context.Context, id uuid.UUID) (*Component, error)
	CreateComponent(ctx context.Context, c *Component) error
	UpdateComponent(ctx context.Context, c *Component) error
	DeleteComponent(ctx context.Context, id uuid.UUID) error
}
