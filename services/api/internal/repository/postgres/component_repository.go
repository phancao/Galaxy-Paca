package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	componentdom "github.com/Paca-AI/api/internal/domain/component"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type componentRecord struct {
	ID           string    `db:"id"`
	ProjectID    string    `db:"project_id"`
	Name         string    `db:"name"`
	Description  *string   `db:"description"`
	LeadMemberID *string   `db:"lead_member_id"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

const componentCols = `id, project_id, name, description, lead_member_id, created_at, updated_at`

// ComponentRepository is the sqlx implementation of componentdom.Repository.
type ComponentRepository struct {
	db *sqlx.DB
}

// NewComponentRepository returns a new ComponentRepository.
func NewComponentRepository(db *sqlx.DB) *ComponentRepository {
	return &ComponentRepository{db: db}
}

// ListComponents returns all components for a project ordered by name.
func (r *ComponentRepository) ListComponents(ctx context.Context, projectID uuid.UUID) ([]*componentdom.Component, error) {
	var records []componentRecord
	if err := r.db.SelectContext(ctx, &records, `SELECT `+componentCols+` FROM components WHERE project_id = $1 ORDER BY name ASC`, projectID.String()); err != nil {
		return nil, fmt.Errorf("component repo: list: %w", err)
	}
	out := make([]*componentdom.Component, 0, len(records))
	for i := range records {
		out = append(out, toComponentEntity(&records[i]))
	}
	return out, nil
}

// FindComponentByID returns the component with the given ID.
func (r *ComponentRepository) FindComponentByID(ctx context.Context, id uuid.UUID) (*componentdom.Component, error) {
	var rec componentRecord
	err := r.db.GetContext(ctx, &rec, `SELECT `+componentCols+` FROM components WHERE id = $1`, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, componentdom.ErrComponentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("component repo: find by id: %w", err)
	}
	return toComponentEntity(&rec), nil
}

// CreateComponent persists a new component.
func (r *ComponentRepository) CreateComponent(ctx context.Context, c *componentdom.Component) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO components (id, project_id, name, description, lead_member_id, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		c.ID.String(), c.ProjectID.String(), c.Name, textToNullable(c.Description),
		uuidPtrToStringPtr(c.LeadMemberID), c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return componentdom.ErrComponentNameTaken
		}
		return fmt.Errorf("component repo: create: %w", err)
	}
	return nil
}

// UpdateComponent persists changes to an existing component.
func (r *ComponentRepository) UpdateComponent(ctx context.Context, c *componentdom.Component) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE components SET name=$1, description=$2, lead_member_id=$3, updated_at=$4
		WHERE id=$5`,
		c.Name, textToNullable(c.Description), uuidPtrToStringPtr(c.LeadMemberID), c.UpdatedAt, c.ID.String(),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return componentdom.ErrComponentNameTaken
		}
		return fmt.Errorf("component repo: update: %w", err)
	}
	return nil
}

// DeleteComponent removes a component by ID.
func (r *ComponentRepository) DeleteComponent(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM components WHERE id = $1`, id.String())
	if err != nil {
		return fmt.Errorf("component repo: delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return componentdom.ErrComponentNotFound
	}
	return nil
}

func toComponentEntity(r *componentRecord) *componentdom.Component {
	id, _ := uuid.Parse(r.ID)
	pid, _ := uuid.Parse(r.ProjectID)
	desc := ""
	if r.Description != nil {
		desc = *r.Description
	}
	return &componentdom.Component{
		ID:           id,
		ProjectID:    pid,
		Name:         r.Name,
		Description:  desc,
		LeadMemberID: stringPtrToUUIDPtr(r.LeadMemberID),
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}
