package dto

import (
	"time"

	componentdom "github.com/Paca-AI/api/internal/domain/component"
	"github.com/google/uuid"
)

// CreateComponentRequest is the body for POST /projects/:projectId/components.
type CreateComponentRequest struct {
	Name         string     `json:"name" binding:"required"`
	Description  string     `json:"description"`
	LeadMemberID *uuid.UUID `json:"lead_member_id"`
}

// UpdateComponentRequest is the body for PATCH /projects/:projectId/components/:componentId.
// Description and LeadMemberID use the Optional* wrappers so an absent field
// leaves the stored value unchanged, distinct from an explicit JSON null.
type UpdateComponentRequest struct {
	Name         string         `json:"name"`
	Description  OptionalString `json:"description"`
	LeadMemberID OptionalUUID   `json:"lead_member_id"`
}

// ComponentResponse is the public representation of a project component.
type ComponentResponse struct {
	ID           uuid.UUID  `json:"id"`
	ProjectID    uuid.UUID  `json:"project_id"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	LeadMemberID *uuid.UUID `json:"lead_member_id,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// ComponentFromEntity maps a domain Component to a ComponentResponse DTO.
func ComponentFromEntity(c *componentdom.Component) ComponentResponse {
	return ComponentResponse{
		ID:           c.ID,
		ProjectID:    c.ProjectID,
		Name:         c.Name,
		Description:  c.Description,
		LeadMemberID: c.LeadMemberID,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
	}
}
