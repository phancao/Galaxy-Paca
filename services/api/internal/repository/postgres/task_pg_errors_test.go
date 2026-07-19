package postgres

import (
	"errors"
	"fmt"
	"testing"

	taskdom "github.com/Paca-AI/api/internal/domain/task"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestTranslateTaskReference(t *testing.T) {
	// A foreign-key violation (e.g. an assignee member id that isn't a project
	// member) is translated into the domain sentinel → 422, not a raw 500.
	fk := &pgconn.PgError{Code: pgForeignKeyViolation, ConstraintName: "task_assignees_member_id_fkey"}
	if got := translateTaskReference(fk); !errors.Is(got, taskdom.ErrTaskReferenceInvalid) {
		t.Fatalf("FK violation should map to ErrTaskReferenceInvalid, got %v", got)
	}
	// Wrapped FK violation (repo layers wrap with fmt.Errorf) is still detected.
	wrapped := fmt.Errorf("task repo: insert assignees: %w", fk)
	if got := translateTaskReference(wrapped); !errors.Is(got, taskdom.ErrTaskReferenceInvalid) {
		t.Fatalf("wrapped FK violation should map to ErrTaskReferenceInvalid, got %v", got)
	}
	// A different SQLSTATE (e.g. unique violation) passes through unchanged.
	unique := &pgconn.PgError{Code: "23505", ConstraintName: "x_key"}
	if got := translateTaskReference(unique); !errors.Is(got, unique) {
		t.Fatalf("non-FK pg error should pass through, got %v", got)
	}
	// A plain error passes through; nil stays nil.
	plain := errors.New("boom")
	if got := translateTaskReference(plain); got != plain {
		t.Fatalf("plain error should pass through, got %v", got)
	}
	if got := translateTaskReference(nil); got != nil {
		t.Fatalf("nil should stay nil, got %v", got)
	}
}
