package postgres

import (
	"errors"
	"fmt"

	taskdom "github.com/Paca-AI/api/internal/domain/task"
	"github.com/jackc/pgx/v5/pgconn"
)

// pgForeignKeyViolation is the SQLSTATE for a foreign-key constraint violation.
const pgForeignKeyViolation = "23503"

// translateTaskReference converts a foreign-key violation raised while writing a
// task into taskdom.ErrTaskReferenceInvalid so the transport layer can answer
// 422 instead of leaking a raw 500. Task writes reference several project-scoped
// entities by id — assignee/reporter member ids, version, component, sprint,
// status, task type — and a bad id (e.g. a caller passing a user id where a
// member id is required) trips one of those FKs. Any other error passes through
// unchanged; a nil error stays nil.
func translateTaskReference(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
		return fmt.Errorf("%w (constraint %s)", taskdom.ErrTaskReferenceInvalid, pgErr.ConstraintName)
	}
	return err
}
