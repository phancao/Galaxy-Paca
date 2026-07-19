package postgres

// Galaxy identity-sync (ADR-040) extensions to UserRepository: lookups that
// see soft-deleted rows and the restore writer used by the Vortex
// user.changed webhook (service/nexussync).  Kept in a separate file so the
// upstream user_repository.go stays rebase-friendly, mirroring
// user_oidc_repository.go.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	userdom "github.com/Paca-AI/api/internal/domain/user"
	"github.com/google/uuid"
)

// FindByOIDCSubIncludingDeleted returns the user linked to the given OIDC
// subject even when the row is soft-deleted, or userdom.ErrNotFound.  Used by
// the ADR-040 restore path: a deprovisioned user's row has deleted_at set, so
// the active-only FindByOIDCSub can never see it again.
func (r *UserRepository) FindByOIDCSubIncludingDeleted(ctx context.Context, sub string) (*userdom.User, error) {
	var row userReadRow
	err := r.db.GetContext(ctx, &row, `
		SELECT `+userReadCols+`
		FROM users
		`+userReadJoin+`
		WHERE users.oidc_sub = $1`, sub)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, userdom.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user repo: find by oidc sub including deleted: %w", err)
	}
	return rowToEntity(&row), nil
}

// Restore reactivates a soft-deleted user by clearing deleted_at — the exact
// inverse of Delete.  A row that is already active is left untouched (the
// write is idempotent, matching Delete's semantics).
func (r *UserRepository) Restore(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `UPDATE users SET deleted_at = NULL, updated_at = $1 WHERE id = $2 AND deleted_at IS NOT NULL`, now, id.String())
	if err != nil {
		return fmt.Errorf("user repo: restore: %w", err)
	}
	return nil
}
