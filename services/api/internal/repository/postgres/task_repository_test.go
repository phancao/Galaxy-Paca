package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	taskdom "github.com/Paca-AI/api/internal/domain/task"
)

// openTaskRepoTestDB sets up an in-memory SQLite DB with a minimal tasks
// table for exercising applyTaskFilter via CountTasks.
func openTaskRepoTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := `
		CREATE TABLE tasks (
			id          TEXT PRIMARY KEY,
			project_id  TEXT NOT NULL,
			deleted_at  DATETIME
		);
		CREATE TABLE task_assignees (
			task_id     TEXT NOT NULL,
			member_id   TEXT NOT NULL,
			assigned_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (task_id, member_id)
		);`
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

// TestTaskRepository_CountTasks_AssigneeNullWithAssigneeIDs is a regression
// test for https://github.com/Paca-AI/paca/issues/272: combining the
// "unassigned" filter with an "assigned to specific users" filter used to
// produce a WHERE clause with unbalanced parentheses, which crashed with a
// SQL syntax error instead of returning matching tasks.
func TestTaskRepository_CountTasks_AssigneeNullWithAssigneeIDs(t *testing.T) {
	db := openTaskRepoTestDB(t)
	repo := NewTaskRepository(db)
	ctx := context.Background()

	projectID := uuid.New()
	assigneeIn := uuid.New()
	assigneeOut := uuid.New()

	taskUnassigned := uuid.New().String()
	taskIn := uuid.New().String()
	taskOut := uuid.New().String()
	db.MustExec(`INSERT INTO tasks (id, project_id) VALUES ($1, $2)`, taskUnassigned, projectID.String())
	db.MustExec(`INSERT INTO tasks (id, project_id) VALUES ($1, $2)`, taskIn, projectID.String())
	db.MustExec(`INSERT INTO tasks (id, project_id) VALUES ($1, $2)`, taskOut, projectID.String())
	db.MustExec(`INSERT INTO task_assignees (task_id, member_id) VALUES ($1, $2)`, taskIn, assigneeIn.String())
	db.MustExec(`INSERT INTO task_assignees (task_id, member_id) VALUES ($1, $2)`, taskOut, assigneeOut.String())

	filter := taskdom.TaskFilter{
		AssigneeNull: true,
		AssigneeIDs:  []uuid.UUID{assigneeIn},
	}

	count, err := repo.CountTasks(ctx, projectID, filter)
	if err != nil {
		t.Fatalf("expected no error combining AssigneeNull with AssigneeIDs, got: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 tasks (unassigned + matching assignee), got %d", count)
	}
}

// TestSyncTaskAssignees_PreservesUnchangedRows is a regression test: UpdateTask
// used to unconditionally DELETE-then-reinsert every task_assignees row on
// every call, which reset assigned_at for every assignee even when an update
// didn't touch assignees at all (e.g. renaming a task), corrupting
// assignment history and making the assigned_at-ordered assignee list
// (used for "first assignee" swimlane grouping and avatar-stack order)
// reorder nondeterministically after unrelated edits. syncTaskAssignees must
// leave rows for members present in both the old and new set untouched, and
// only remove/insert the actual diff.
func TestSyncTaskAssignees_PreservesUnchangedRows(t *testing.T) {
	db := openTaskRepoTestDB(t)
	ctx := context.Background()

	taskID := uuid.New()
	memberA := uuid.New()
	memberB := uuid.New()
	memberC := uuid.New()

	db.MustExec(`INSERT INTO tasks (id, project_id) VALUES ($1, $2)`, taskID.String(), uuid.New().String())
	db.MustExec(`INSERT INTO task_assignees (task_id, member_id, assigned_at) VALUES ($1, $2, $3)`,
		taskID.String(), memberA.String(), "2020-01-01T00:00:00Z")
	db.MustExec(`INSERT INTO task_assignees (task_id, member_id, assigned_at) VALUES ($1, $2, $3)`,
		taskID.String(), memberB.String(), "2020-01-02T00:00:00Z")

	type row struct {
		MemberID   string `db:"member_id"`
		AssignedAt string `db:"assigned_at"`
	}
	readRows := func() map[string]string {
		t.Helper()
		var rows []row
		if err := db.SelectContext(ctx, &rows, `SELECT member_id, assigned_at FROM task_assignees WHERE task_id=$1`, taskID.String()); err != nil {
			t.Fatalf("query rows: %v", err)
		}
		out := make(map[string]string, len(rows))
		for _, r := range rows {
			out[r.MemberID] = r.AssignedAt
		}
		return out
	}

	// A no-op sync (wantIDs identical to the current set, simulating an
	// update that doesn't touch assignee_ids at all) must not touch
	// assigned_at for either existing member.
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := syncTaskAssignees(ctx, tx, taskID, []uuid.UUID{memberA, memberB}); err != nil {
		t.Fatalf("syncTaskAssignees (no-op): %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got := readRows()
	if len(got) != 2 {
		t.Fatalf("expected 2 assignee rows to survive a no-op sync, got %+v", got)
	}
	if got[memberA.String()] != "2020-01-01T00:00:00Z" || got[memberB.String()] != "2020-01-02T00:00:00Z" {
		t.Fatalf("expected assigned_at to be preserved by a no-op sync, got %+v", got)
	}

	// A real change (drop B, add C) must preserve A's original assigned_at,
	// remove B, and add C.
	tx2, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := syncTaskAssignees(ctx, tx2, taskID, []uuid.UUID{memberA, memberC}); err != nil {
		t.Fatalf("syncTaskAssignees (change): %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got = readRows()
	if _, stillPresent := got[memberB.String()]; stillPresent {
		t.Fatalf("expected memberB to be removed after dropping it from wantIDs, got %+v", got)
	}
	if got[memberA.String()] != "2020-01-01T00:00:00Z" {
		t.Fatalf("expected memberA's original assigned_at to survive an unrelated diff, got %q", got[memberA.String()])
	}
	if _, added := got[memberC.String()]; !added {
		t.Fatalf("expected memberC to be newly inserted, got %+v", got)
	}
}
