package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	versiondom "github.com/Paca-AI/api/internal/domain/version"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type versionRecord struct {
	ID          string     `db:"id"`
	ProjectID   string     `db:"project_id"`
	Name        string     `db:"name"`
	Description *string    `db:"description"`
	Released    bool       `db:"released"`
	ReleaseDate *time.Time `db:"release_date"`
	Archived    bool       `db:"archived"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
}

const versionCols = `id, project_id, name, description, released, release_date, archived, created_at, updated_at`

// VersionRepository is the sqlx implementation of versiondom.Repository.
type VersionRepository struct {
	db *sqlx.DB
}

// NewVersionRepository returns a new VersionRepository.
func NewVersionRepository(db *sqlx.DB) *VersionRepository {
	return &VersionRepository{db: db}
}

// ListVersions returns all versions for a project ordered by creation time.
func (r *VersionRepository) ListVersions(ctx context.Context, projectID uuid.UUID) ([]*versiondom.Version, error) {
	var records []versionRecord
	if err := r.db.SelectContext(ctx, &records, `SELECT `+versionCols+` FROM versions WHERE project_id = $1 ORDER BY created_at ASC`, projectID.String()); err != nil {
		return nil, fmt.Errorf("version repo: list: %w", err)
	}
	out := make([]*versiondom.Version, 0, len(records))
	for i := range records {
		out = append(out, toVersionEntity(&records[i]))
	}
	return out, nil
}

// FindVersionByID returns the version with the given ID.
func (r *VersionRepository) FindVersionByID(ctx context.Context, id uuid.UUID) (*versiondom.Version, error) {
	var rec versionRecord
	err := r.db.GetContext(ctx, &rec, `SELECT `+versionCols+` FROM versions WHERE id = $1`, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, versiondom.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("version repo: find by id: %w", err)
	}
	return toVersionEntity(&rec), nil
}

// CreateVersion persists a new version.
func (r *VersionRepository) CreateVersion(ctx context.Context, v *versiondom.Version) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO versions (id, project_id, name, description, released, release_date, archived, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		v.ID.String(), v.ProjectID.String(), v.Name, textToNullable(v.Description),
		v.Released, v.ReleaseDate, v.Archived, v.CreatedAt, v.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return versiondom.ErrVersionNameTaken
		}
		return fmt.Errorf("version repo: create: %w", err)
	}
	return nil
}

// UpdateVersion persists changes to an existing version.
func (r *VersionRepository) UpdateVersion(ctx context.Context, v *versiondom.Version) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE versions SET name=$1, description=$2, released=$3, release_date=$4, archived=$5, updated_at=$6
		WHERE id=$7`,
		v.Name, textToNullable(v.Description), v.Released, v.ReleaseDate, v.Archived, v.UpdatedAt, v.ID.String(),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return versiondom.ErrVersionNameTaken
		}
		return fmt.Errorf("version repo: update: %w", err)
	}
	return nil
}

// DeleteVersion removes a version by ID.
func (r *VersionRepository) DeleteVersion(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM versions WHERE id = $1`, id.String())
	if err != nil {
		return fmt.Errorf("version repo: delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return versiondom.ErrVersionNotFound
	}
	return nil
}

func toVersionEntity(r *versionRecord) *versiondom.Version {
	id, _ := uuid.Parse(r.ID)
	pid, _ := uuid.Parse(r.ProjectID)
	desc := ""
	if r.Description != nil {
		desc = *r.Description
	}
	return &versiondom.Version{
		ID:          id,
		ProjectID:   pid,
		Name:        r.Name,
		Description: desc,
		Released:    r.Released,
		ReleaseDate: r.ReleaseDate,
		Archived:    r.Archived,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

// textToNullable maps an empty string to a SQL NULL and any non-empty string to
// itself, so an absent/cleared text column stores NULL rather than "".
func textToNullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
