// Package versionsvc implements the project version (fixVersion) application service.
package versionsvc

import (
	"context"
	"strings"
	"time"

	versiondom "github.com/Paca-AI/api/internal/domain/version"
	"github.com/google/uuid"
)

// Service is the concrete implementation of versiondom.Service.
type Service struct {
	repo versiondom.Repository
}

// New returns a configured version service.
func New(repo versiondom.Repository) *Service {
	return &Service{repo: repo}
}

// ListVersions returns all versions for a project.
func (s *Service) ListVersions(ctx context.Context, projectID uuid.UUID) ([]*versiondom.Version, error) {
	return s.repo.ListVersions(ctx, projectID)
}

// CreateVersion creates a new version for the given project.
func (s *Service) CreateVersion(ctx context.Context, in versiondom.CreateVersionInput) (*versiondom.Version, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, versiondom.ErrVersionNameInvalid
	}

	now := time.Now()
	v := &versiondom.Version{
		ID:          uuid.New(),
		ProjectID:   in.ProjectID,
		Name:        name,
		Description: strings.TrimSpace(in.Description),
		Released:    in.Released,
		ReleaseDate: in.ReleaseDate,
		Archived:    in.Archived,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.CreateVersion(ctx, v); err != nil {
		return nil, err
	}
	return v, nil
}

// UpdateVersion updates the mutable fields of an existing version.
func (s *Service) UpdateVersion(ctx context.Context, projectID, id uuid.UUID, in versiondom.UpdateVersionInput) (*versiondom.Version, error) {
	v, err := s.repo.FindVersionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if v.ProjectID != projectID {
		return nil, versiondom.ErrVersionNotFound
	}

	if name := strings.TrimSpace(in.Name); name != "" {
		v.Name = name
	}
	if in.Description != nil {
		if *in.Description != nil {
			v.Description = strings.TrimSpace(**in.Description)
		} else {
			v.Description = ""
		}
	}
	if in.Released != nil {
		v.Released = *in.Released
	}
	if in.ReleaseDate != nil {
		v.ReleaseDate = *in.ReleaseDate
	}
	if in.Archived != nil {
		v.Archived = *in.Archived
	}
	v.UpdatedAt = time.Now()

	if err := s.repo.UpdateVersion(ctx, v); err != nil {
		return nil, err
	}
	return v, nil
}

// DeleteVersion removes a version by ID, verifying it belongs to projectID.
func (s *Service) DeleteVersion(ctx context.Context, projectID, id uuid.UUID) error {
	v, err := s.repo.FindVersionByID(ctx, id)
	if err != nil {
		return err
	}
	if v.ProjectID != projectID {
		return versiondom.ErrVersionNotFound
	}
	return s.repo.DeleteVersion(ctx, id)
}
