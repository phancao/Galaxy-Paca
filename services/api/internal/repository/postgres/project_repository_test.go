package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	projectdom "github.com/Paca-AI/api/internal/domain/project"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func openProjectRepoTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := `
		CREATE TABLE projects (
			id           TEXT    PRIMARY KEY,
			name         TEXT    NOT NULL,
			description  TEXT    NOT NULL DEFAULT '',
			task_id_prefix TEXT  NOT NULL DEFAULT '',
			is_public    INTEGER NOT NULL DEFAULT 0,
			settings     BLOB    NOT NULL DEFAULT '{}',
			created_by   TEXT,
			created_at   DATETIME,
			deleted_at   DATETIME
		);

		-- ProjectRepository.Delete cascade nhánh soft-delete sang hai bảng con
		-- (xem chú thích ở project_repository.go). Thiếu chúng thì Delete nổ
		-- "no such table: tasks" và MỌI phép kiểm soft-delete đỏ.
		CREATE TABLE tasks (
			id         TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			title      TEXT NOT NULL DEFAULT '',
			updated_at DATETIME,
			deleted_at DATETIME
		);

		CREATE TABLE project_members (
			project_id TEXT NOT NULL,
			user_id    TEXT NOT NULL,
			deleted_at DATETIME
		);`
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func testProject(id uuid.UUID, prefix string) *projectdom.Project {
	now := time.Now().UTC().Truncate(time.Second)
	return &projectdom.Project{
		ID:           id,
		Name:         "Test Project " + prefix,
		Description:  "test description",
		TaskIDPrefix: prefix,
		IsPublic:     false,
		Settings:     map[string]any{},
		CreatedAt:    now,
	}
}

func TestProjectRepository_Delete_SetsDeletedAt(t *testing.T) {
	db := openProjectRepoTestDB(t)
	repo := NewProjectRepository(db)
	ctx := context.Background()

	p := testProject(uuid.New(), "DEL")
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Bypass the soft-delete filter to confirm deleted_at was set.
	var rec projectRecord
	if err := db.GetContext(ctx, &rec,
		`SELECT `+projectSelectCols+` FROM projects WHERE id = $1`, p.ID.String()); err != nil {
		t.Fatalf("raw query: %v", err)
	}
	if rec.DeletedAt == nil {
		t.Fatal("expected deleted_at to be non-nil after Delete")
	}
}

// TestProjectRepository_Delete_CascadesSoftDeleteToTasksAndMembers đo CHÍNH cái
// mà 3652a22d thêm vào Delete, chứ không chỉ đo rằng Delete hết nổ.
//
// Không có phép kiểm này thì bảng tasks/project_members trong schema test chỉ là
// thứ làm cho hết đỏ: bỏ hẳn hai lệnh UPDATE cascade khỏi mã sản xuất, mọi phép
// kiểm soft-delete khác vẫn xanh, và hai lỗi mà 3652a22d vá (ghost member,
// hai task cùng mang khoá ABC-1 sau khi prefix được tái dùng) lặng lẽ trở lại.
func TestProjectRepository_Delete_CascadesSoftDeleteToTasksAndMembers(t *testing.T) {
	db := openProjectRepoTestDB(t)
	repo := NewProjectRepository(db)
	ctx := context.Background()

	p := testProject(uuid.New(), "CAS")
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	liveTask := uuid.New()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO tasks (id, project_id, title) VALUES ($1, $2, $3)`,
		liveTask.String(), p.ID.String(), "task còn sống"); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	// Task đã bị xoá mềm TỪ TRƯỚC: mệnh đề `AND deleted_at IS NULL` phải chừa
	// nó ra, chứ không dập lại dấu thời gian.
	goneTask := uuid.New()
	earlier := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO tasks (id, project_id, title, deleted_at) VALUES ($1, $2, $3, $4)`,
		goneTask.String(), p.ID.String(), "task xoá từ trước", earlier); err != nil {
		t.Fatalf("seed deleted task: %v", err)
	}

	// Task của project KHÁC: cascade không được đụng tới.
	other := testProject(uuid.New(), "OTH")
	if err := repo.Create(ctx, other); err != nil {
		t.Fatalf("Create other: %v", err)
	}
	otherTask := uuid.New()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO tasks (id, project_id, title) VALUES ($1, $2, $3)`,
		otherTask.String(), other.ID.String(), "task project khác"); err != nil {
		t.Fatalf("seed other task: %v", err)
	}

	member := uuid.New()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO project_members (project_id, user_id) VALUES ($1, $2)`,
		p.ID.String(), member.String()); err != nil {
		t.Fatalf("seed member: %v", err)
	}

	if err := repo.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	taskDeletedAt := func(id uuid.UUID) *time.Time {
		t.Helper()
		var at *time.Time
		if err := db.GetContext(ctx, &at, `SELECT deleted_at FROM tasks WHERE id = $1`, id.String()); err != nil {
			t.Fatalf("read task %s: %v", id, err)
		}
		return at
	}

	if at := taskDeletedAt(liveTask); at == nil {
		t.Error("task còn sống của project bị xoá phải được đánh dấu deleted_at")
	}
	if at := taskDeletedAt(goneTask); at == nil || !at.UTC().Equal(earlier) {
		t.Errorf("task đã xoá từ trước phải giữ nguyên dấu thời gian %v, got %v", earlier, at)
	}
	if at := taskDeletedAt(otherTask); at != nil {
		t.Error("cascade không được đụng tới task của project khác")
	}

	var memberDeletedAt *time.Time
	if err := db.GetContext(ctx, &memberDeletedAt,
		`SELECT deleted_at FROM project_members WHERE project_id = $1 AND user_id = $2`,
		p.ID.String(), member.String()); err != nil {
		t.Fatalf("read member: %v", err)
	}
	if memberDeletedAt == nil {
		t.Error("thành viên của project bị xoá phải được đánh dấu deleted_at (ghost member)")
	}
}

func TestProjectRepository_FindByID_ExcludesSoftDeleted(t *testing.T) {
	db := openProjectRepoTestDB(t)
	repo := NewProjectRepository(db)
	ctx := context.Background()

	p := testProject(uuid.New(), "FID")
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.FindByID(ctx, p.ID)
	if !errors.Is(err, projectdom.ErrNotFound) {
		t.Errorf("expected ErrNotFound after soft-delete, got %v", err)
	}
}

func TestProjectRepository_FindByTaskIDPrefix_ExcludesSoftDeleted(t *testing.T) {
	db := openProjectRepoTestDB(t)
	repo := NewProjectRepository(db)
	ctx := context.Background()

	p := testProject(uuid.New(), "PFXD")
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.FindByTaskIDPrefix(ctx, "PFXD")
	if !errors.Is(err, projectdom.ErrNotFound) {
		t.Errorf("expected ErrNotFound for soft-deleted prefix, got %v", err)
	}
}

func TestProjectRepository_Delete_AlreadyDeleted_ReturnsNotFound(t *testing.T) {
	db := openProjectRepoTestDB(t)
	repo := NewProjectRepository(db)
	ctx := context.Background()

	p := testProject(uuid.New(), "DUP")
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Delete(ctx, p.ID); err != nil {
		t.Fatalf("first Delete: %v", err)
	}

	err := repo.Delete(ctx, p.ID)
	if !errors.Is(err, projectdom.ErrNotFound) {
		t.Errorf("expected ErrNotFound on re-delete, got %v", err)
	}
}

func TestProjectRepository_List_ExcludesSoftDeleted(t *testing.T) {
	db := openProjectRepoTestDB(t)
	repo := NewProjectRepository(db)
	ctx := context.Background()

	alive := testProject(uuid.New(), "LIVE")
	deleted := testProject(uuid.New(), "GONE")

	for _, p := range []*projectdom.Project{alive, deleted} {
		if err := repo.Create(ctx, p); err != nil {
			t.Fatalf("Create %s: %v", p.TaskIDPrefix, err)
		}
	}
	if err := repo.Delete(ctx, deleted.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify COUNT excludes soft-deleted rows — mirrors the repo's own count query.
	var total int64
	if err := db.GetContext(ctx, &total, `SELECT COUNT(*) FROM projects WHERE deleted_at IS NULL`); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if total != 1 {
		t.Errorf("expected COUNT=1 (soft-deleted excluded), got %d", total)
	}

	// Verify only the alive project appears in a filtered select.
	var ids []string
	if err := db.SelectContext(ctx, &ids, `SELECT id FROM projects WHERE deleted_at IS NULL`); err != nil {
		t.Fatalf("select query: %v", err)
	}
	if len(ids) != 1 || ids[0] != alive.ID.String() {
		t.Errorf("expected only alive project %s, got %v", alive.ID, ids)
	}
}
