package dto

import (
	"time"

	versiondom "github.com/Paca-AI/api/internal/domain/version"
	"github.com/google/uuid"
)

// CreateVersionRequest is the body for POST /projects/:projectId/versions.
type CreateVersionRequest struct {
	Name        string     `json:"name" binding:"required"`
	Description string     `json:"description"`
	Released    bool       `json:"released"`
	ReleaseDate *time.Time `json:"release_date"`
	Archived    bool       `json:"archived"`
}

// UpdateVersionRequest is the body for PATCH /projects/:projectId/versions/:versionId.
// Description and ReleaseDate use the Optional* wrappers so an absent field
// leaves the stored value unchanged, distinct from an explicit JSON null.
type UpdateVersionRequest struct {
	Name        string         `json:"name"`
	Description OptionalString `json:"description"`
	Released    *bool          `json:"released"`
	ReleaseDate OptionalTime   `json:"release_date"`
	Archived    *bool          `json:"archived"`
}

// VersionResponse is the public representation of a project version.
type VersionResponse struct {
	ID          uuid.UUID  `json:"id"`
	ProjectID   uuid.UUID  `json:"project_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Released    bool       `json:"released"`
	ReleaseDate *time.Time `json:"release_date,omitempty"`
	Archived    bool       `json:"archived"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// VersionFromEntity maps a domain Version to a VersionResponse DTO.
func VersionFromEntity(v *versiondom.Version) VersionResponse {
	return VersionResponse{
		ID:          v.ID,
		ProjectID:   v.ProjectID,
		Name:        v.Name,
		Description: v.Description,
		Released:    v.Released,
		ReleaseDate: v.ReleaseDate,
		Archived:    v.Archived,
		CreatedAt:   v.CreatedAt,
		UpdatedAt:   v.UpdatedAt,
	}
}
