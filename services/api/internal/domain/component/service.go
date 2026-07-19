package componentdom

import (
	"context"

	"github.com/google/uuid"
)

// Service defines project component use cases.
type Service interface {
	ListComponents(ctx context.Context, projectID uuid.UUID) ([]*Component, error)
	CreateComponent(ctx context.Context, in CreateComponentInput) (*Component, error)
	// UpdateComponent updates the component identified by id, verifying it belongs to projectID.
	UpdateComponent(ctx context.Context, projectID, id uuid.UUID, in UpdateComponentInput) (*Component, error)
	// DeleteComponent removes the component identified by id, verifying it belongs to projectID.
	DeleteComponent(ctx context.Context, projectID, id uuid.UUID) error
}

// CreateComponentInput carries fields required to create a component.
type CreateComponentInput struct {
	ProjectID    uuid.UUID
	Name         string
	Description  string
	LeadMemberID *uuid.UUID
}

// UpdateComponentInput carries mutable component fields for a PATCH operation.
// Name is applied when non-empty. Description and LeadMemberID use the
// double-pointer convention (nil = absent, &nil = clear, &&value = set).
type UpdateComponentInput struct {
	Name         string
	Description  **string
	LeadMemberID **uuid.UUID
}
