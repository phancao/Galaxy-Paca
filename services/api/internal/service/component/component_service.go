// Package componentsvc implements the project component application service.
package componentsvc

import (
	"context"
	"strings"
	"time"

	componentdom "github.com/Paca-AI/api/internal/domain/component"
	"github.com/google/uuid"
)

// Service is the concrete implementation of componentdom.Service.
type Service struct {
	repo componentdom.Repository
}

// New returns a configured component service.
func New(repo componentdom.Repository) *Service {
	return &Service{repo: repo}
}

// ListComponents returns all components for a project.
func (s *Service) ListComponents(ctx context.Context, projectID uuid.UUID) ([]*componentdom.Component, error) {
	return s.repo.ListComponents(ctx, projectID)
}

// CreateComponent creates a new component for the given project.
func (s *Service) CreateComponent(ctx context.Context, in componentdom.CreateComponentInput) (*componentdom.Component, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, componentdom.ErrComponentNameInvalid
	}

	now := time.Now()
	c := &componentdom.Component{
		ID:           uuid.New(),
		ProjectID:    in.ProjectID,
		Name:         name,
		Description:  strings.TrimSpace(in.Description),
		LeadMemberID: in.LeadMemberID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.CreateComponent(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// UpdateComponent updates the mutable fields of an existing component.
func (s *Service) UpdateComponent(ctx context.Context, projectID, id uuid.UUID, in componentdom.UpdateComponentInput) (*componentdom.Component, error) {
	c, err := s.repo.FindComponentByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.ProjectID != projectID {
		return nil, componentdom.ErrComponentNotFound
	}

	if name := strings.TrimSpace(in.Name); name != "" {
		c.Name = name
	}
	if in.Description != nil {
		if *in.Description != nil {
			c.Description = strings.TrimSpace(**in.Description)
		} else {
			c.Description = ""
		}
	}
	if in.LeadMemberID != nil {
		c.LeadMemberID = *in.LeadMemberID
	}
	c.UpdatedAt = time.Now()

	if err := s.repo.UpdateComponent(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// DeleteComponent removes a component by ID, verifying it belongs to projectID.
func (s *Service) DeleteComponent(ctx context.Context, projectID, id uuid.UUID) error {
	c, err := s.repo.FindComponentByID(ctx, id)
	if err != nil {
		return err
	}
	if c.ProjectID != projectID {
		return componentdom.ErrComponentNotFound
	}
	return s.repo.DeleteComponent(ctx, id)
}
