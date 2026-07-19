package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	worklogdom "github.com/Paca-AI/api/internal/domain/worklog"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type worklogRecord struct {
	ID        string    `db:"id"`
	TaskID    string    `db:"task_id"`
	MemberID  *string   `db:"member_id"`
	Minutes   int       `db:"minutes"`
	Note      *string   `db:"note"`
	LoggedAt  time.Time `db:"logged_at"`
	CreatedAt time.Time `db:"created_at"`
}

const worklogCols = `id, task_id, member_id, minutes, note, logged_at, created_at`

// WorklogRepository is the sqlx implementation of worklogdom.Repository.
type WorklogRepository struct {
	db *sqlx.DB
}

// NewWorklogRepository returns a new WorklogRepository.
func NewWorklogRepository(db *sqlx.DB) *WorklogRepository {
	return &WorklogRepository{db: db}
}

// ListWorklogs returns all worklogs for a task ordered by logged_at ascending.
func (r *WorklogRepository) ListWorklogs(ctx context.Context, taskID uuid.UUID) ([]*worklogdom.Worklog, error) {
	var records []worklogRecord
	if err := r.db.SelectContext(ctx, &records, `SELECT `+worklogCols+` FROM task_worklogs WHERE task_id = $1 ORDER BY logged_at ASC, created_at ASC`, taskID.String()); err != nil {
		return nil, fmt.Errorf("worklog repo: list: %w", err)
	}
	out := make([]*worklogdom.Worklog, 0, len(records))
	for i := range records {
		out = append(out, toWorklogEntity(&records[i]))
	}
	return out, nil
}

// ListProjectWorklogs returns all worklogs across a project's tasks matching the
// filter, ordered by logged_at ascending. It joins task_worklogs to tasks to
// scope by project and to honor the optional date-range / member constraints.
func (r *WorklogRepository) ListProjectWorklogs(ctx context.Context, projectID uuid.UUID, filter worklogdom.WorklogFilter) ([]*worklogdom.Worklog, error) {
	query := `SELECT w.id, w.task_id, w.member_id, w.minutes, w.note, w.logged_at, w.created_at
	          FROM task_worklogs w
	          JOIN tasks t ON t.id = w.task_id
	          WHERE t.project_id = $1`
	args := []any{projectID.String()}
	if filter.From != nil {
		args = append(args, *filter.From)
		query += fmt.Sprintf(" AND w.logged_at >= $%d", len(args))
	}
	if filter.To != nil {
		args = append(args, *filter.To)
		query += fmt.Sprintf(" AND w.logged_at <= $%d", len(args))
	}
	if filter.MemberID != nil {
		args = append(args, filter.MemberID.String())
		query += fmt.Sprintf(" AND w.member_id = $%d", len(args))
	}
	query += " ORDER BY w.logged_at ASC, w.created_at ASC"

	var records []worklogRecord
	if err := r.db.SelectContext(ctx, &records, query, args...); err != nil {
		return nil, fmt.Errorf("worklog repo: list by project: %w", err)
	}
	out := make([]*worklogdom.Worklog, 0, len(records))
	for i := range records {
		out = append(out, toWorklogEntity(&records[i]))
	}
	return out, nil
}

// FindWorklogByID returns the worklog with the given ID.
func (r *WorklogRepository) FindWorklogByID(ctx context.Context, id uuid.UUID) (*worklogdom.Worklog, error) {
	var rec worklogRecord
	err := r.db.GetContext(ctx, &rec, `SELECT `+worklogCols+` FROM task_worklogs WHERE id = $1`, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, worklogdom.ErrWorklogNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("worklog repo: find by id: %w", err)
	}
	return toWorklogEntity(&rec), nil
}

// CreateWorklog persists a new worklog.
func (r *WorklogRepository) CreateWorklog(ctx context.Context, w *worklogdom.Worklog) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO task_worklogs (id, task_id, member_id, minutes, note, logged_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		w.ID.String(), w.TaskID.String(), uuidPtrToStringPtr(w.MemberID),
		w.Minutes, textToNullable(w.Note), w.LoggedAt, w.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("worklog repo: create: %w", err)
	}
	return nil
}

// DeleteWorklog removes a worklog by ID.
func (r *WorklogRepository) DeleteWorklog(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM task_worklogs WHERE id = $1`, id.String())
	if err != nil {
		return fmt.Errorf("worklog repo: delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return worklogdom.ErrWorklogNotFound
	}
	return nil
}

func toWorklogEntity(r *worklogRecord) *worklogdom.Worklog {
	id, _ := uuid.Parse(r.ID)
	taskID, _ := uuid.Parse(r.TaskID)
	note := ""
	if r.Note != nil {
		note = *r.Note
	}
	return &worklogdom.Worklog{
		ID:        id,
		TaskID:    taskID,
		MemberID:  stringPtrToUUIDPtr(r.MemberID),
		Minutes:   r.Minutes,
		Note:      note,
		LoggedAt:  r.LoggedAt,
		CreatedAt: r.CreatedAt,
	}
}
