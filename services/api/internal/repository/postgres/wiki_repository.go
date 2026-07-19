package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	wikispacedom "github.com/Paca-AI/api/internal/domain/wikispace"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type projectWikiSpaceRecord struct {
	ProjectID    string    `db:"project_id"`
	WikiFolderID string    `db:"wiki_folder_id"`
	WikiTeamID   string    `db:"wiki_team_id"`
	WikiURL      string    `db:"wiki_url"`
	CreatedBy    *string   `db:"created_by"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

type taskWikiLinkRecord struct {
	ID           string    `db:"id"`
	TaskID       string    `db:"task_id"`
	WikiRecordID string    `db:"wiki_record_id"`
	WikiURL      string    `db:"wiki_url"`
	Title        string    `db:"title"`
	CreatedBy    *string   `db:"created_by"`
	CreatedAt    time.Time `db:"created_at"`
}

// WikiRepository is the sqlx implementation of wikispacedom.Repository.
type WikiRepository struct {
	db *sqlx.DB
}

// NewWikiRepository returns a new WikiRepository.
func NewWikiRepository(db *sqlx.DB) *WikiRepository {
	return &WikiRepository{db: db}
}

// GetSpace returns the project's wiki space mapping.
func (r *WikiRepository) GetSpace(ctx context.Context, projectID uuid.UUID) (*wikispacedom.ProjectWikiSpace, error) {
	var rec projectWikiSpaceRecord
	err := r.db.GetContext(ctx, &rec,
		`SELECT project_id, wiki_folder_id, wiki_team_id, wiki_url, created_by, created_at, updated_at
		 FROM project_wiki_spaces WHERE project_id = $1`, projectID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, wikispacedom.ErrSpaceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("wiki repo: get space: %w", err)
	}
	return spaceFromRecord(&rec), nil
}

// SaveSpace upserts the project's wiki space mapping.
func (r *WikiRepository) SaveSpace(ctx context.Context, s *wikispacedom.ProjectWikiSpace) error {
	var createdBy *string
	if s.CreatedBy != nil {
		v := s.CreatedBy.String()
		createdBy = &v
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO project_wiki_spaces (project_id, wiki_folder_id, wiki_team_id, wiki_url, created_by)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (project_id) DO UPDATE SET
		   wiki_folder_id = EXCLUDED.wiki_folder_id,
		   wiki_team_id   = EXCLUDED.wiki_team_id,
		   wiki_url       = EXCLUDED.wiki_url,
		   updated_at     = now()`,
		s.ProjectID.String(), s.WikiFolderID, s.WikiTeamID, s.WikiURL, createdBy)
	if err != nil {
		return fmt.Errorf("wiki repo: save space: %w", err)
	}
	return nil
}

// ListTaskLinks returns a task's wiki links, newest first.
func (r *WikiRepository) ListTaskLinks(ctx context.Context, taskID uuid.UUID) ([]*wikispacedom.TaskWikiLink, error) {
	var records []taskWikiLinkRecord
	err := r.db.SelectContext(ctx, &records,
		`SELECT id, task_id, wiki_record_id, wiki_url, title, created_by, created_at
		 FROM task_wiki_links WHERE task_id = $1 ORDER BY created_at DESC`, taskID.String())
	if err != nil {
		return nil, fmt.Errorf("wiki repo: list task links: %w", err)
	}
	links := make([]*wikispacedom.TaskWikiLink, 0, len(records))
	for i := range records {
		links = append(links, linkFromRecord(&records[i]))
	}
	return links, nil
}

// AddTaskLink upserts a task↔record link.
func (r *WikiRepository) AddTaskLink(ctx context.Context, l *wikispacedom.TaskWikiLink) error {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	var createdBy *string
	if l.CreatedBy != nil {
		v := l.CreatedBy.String()
		createdBy = &v
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO task_wiki_links (id, task_id, wiki_record_id, wiki_url, title, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (task_id, wiki_record_id) DO UPDATE SET
		   wiki_url = EXCLUDED.wiki_url,
		   title    = EXCLUDED.title`,
		l.ID.String(), l.TaskID.String(), l.WikiRecordID, l.WikiURL, l.Title, createdBy)
	if err != nil {
		return fmt.Errorf("wiki repo: add task link: %w", err)
	}
	return nil
}

// RemoveTaskLink deletes one link.
func (r *WikiRepository) RemoveTaskLink(ctx context.Context, taskID uuid.UUID, wikiRecordID string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM task_wiki_links WHERE task_id = $1 AND wiki_record_id = $2`,
		taskID.String(), wikiRecordID)
	if err != nil {
		return fmt.Errorf("wiki repo: remove task link: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return wikispacedom.ErrLinkNotFound
	}
	return nil
}

func spaceFromRecord(rec *projectWikiSpaceRecord) *wikispacedom.ProjectWikiSpace {
	s := &wikispacedom.ProjectWikiSpace{
		ProjectID:    uuid.MustParse(rec.ProjectID),
		WikiFolderID: rec.WikiFolderID,
		WikiTeamID:   rec.WikiTeamID,
		WikiURL:      rec.WikiURL,
		CreatedAt:    rec.CreatedAt,
		UpdatedAt:    rec.UpdatedAt,
	}
	if rec.CreatedBy != nil {
		if id, err := uuid.Parse(*rec.CreatedBy); err == nil {
			s.CreatedBy = &id
		}
	}
	return s
}

func linkFromRecord(rec *taskWikiLinkRecord) *wikispacedom.TaskWikiLink {
	l := &wikispacedom.TaskWikiLink{
		ID:           uuid.MustParse(rec.ID),
		TaskID:       uuid.MustParse(rec.TaskID),
		WikiRecordID: rec.WikiRecordID,
		WikiURL:      rec.WikiURL,
		Title:        rec.Title,
		CreatedAt:    rec.CreatedAt,
	}
	if rec.CreatedBy != nil {
		if id, err := uuid.Parse(*rec.CreatedBy); err == nil {
			l.CreatedBy = &id
		}
	}
	return l
}
