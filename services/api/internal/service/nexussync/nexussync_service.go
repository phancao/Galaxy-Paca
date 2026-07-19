// Package nexussync applies Vortex→Paca identity-sync events (ADR-040 §3.3).
//
// The Vortex identity service is authoritative for every user Paca mirrors
// (ADR-040 §2); this service converges the local users table on the
// user.changed lifecycle signal: status != "active" or deleted == true is the
// deprovision signal (deactivate the mirror row and cut live sessions), and a
// later status == "active" restores the row.  Group / grant / app-admin
// events do not apply — Paca mirrors no groups, dataset grants, or per-app
// admin flags — and are acknowledged by the transport layer without reaching
// this service.
package nexussync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	userdom "github.com/Paca-AI/api/internal/domain/user"
	"github.com/google/uuid"
)

// UserStore is the persistence contract nexussync needs.  It is implemented
// by the postgres UserRepository (user_repository.go + the ADR-040 extensions
// in user_nexus_repository.go).
type UserStore interface {
	FindByOIDCSub(ctx context.Context, sub string) (*userdom.User, error)
	FindByEmail(ctx context.Context, email string) (*userdom.User, error)
	FindByOIDCSubIncludingDeleted(ctx context.Context, sub string) (*userdom.User, error)
	// Delete soft-deletes (sets deleted_at) — the same mechanism the admin
	// DELETE /admin/users/{id} path uses; Paca has no separate status column.
	Delete(ctx context.Context, id uuid.UUID) error
	// Restore clears deleted_at, reversing Delete.
	Restore(ctx context.Context, id uuid.UUID) error
}

// UserChanged carries the ADR-040 §3.2 user.changed payload.  UserID is the
// Vortex user id — the same value stored in users.oidc_sub by the OIDC SSO
// login and directory-sync paths.
type UserChanged struct {
	UserID  string
	Email   string
	Status  string
	Deleted bool
}

// Outcome describes what applying an event did; the transport layer echoes it
// verbatim in the webhook response body.
type Outcome string

// Outcomes of ApplyUserChanged.
const (
	OutcomeDeprovisioned  Outcome = "deprovisioned"
	OutcomeRestored       Outcome = "restored"
	OutcomeNotProvisioned Outcome = "not provisioned"
	OutcomeNoChange       Outcome = "no change"
)

// Service applies identity-sync events to the local user mirror.
type Service struct {
	users UserStore
	log   *slog.Logger
}

// New returns a configured nexussync Service.
func New(users UserStore, log *slog.Logger) *Service {
	return &Service{users: users, log: log}
}

// ApplyUserChanged converges the local mirror on a verified user.changed
// event: deprovision on deleted/non-active status, restore on active.
func (s *Service) ApplyUserChanged(ctx context.Context, evt UserChanged) (Outcome, error) {
	if evt.UserID == "" {
		return "", fmt.Errorf("nexussync: user.changed event has no user_id")
	}
	if evt.Deleted || evt.Status != "active" {
		return s.deprovision(ctx, evt)
	}
	return s.restore(ctx, evt)
}

// deprovision deactivates the mirrored user via the soft-delete flag —
// deleted_at is Paca's only deactivation mechanism (there is no status/active
// column), and it is exactly what the admin DELETE path sets, so every
// active-only query (login, JIT resolution, listings) stops seeing the row
// at once.
//
// Session kill (ADR-040 §3.3.3b): Paca mints its own stateless JWT session
// pair (auth.Service.IssueSession).  Setting deleted_at cuts the refresh path
// immediately — Refresh re-loads the user via FindByID, which filters
// deleted_at IS NULL, so every subsequent rotation fails with
// ErrSessionInvalidated — and Vortex RS256 bearer tokens die the same way
// (galaxyauth resolves them per-request through FindByOIDCSub, same filter).
// The already-issued *access* token cannot be revoked server-side: the
// redis RefreshTokenStore is keyed by session familyID/jti with no
// user→family index, and access-token verification (middleware.Authn) is
// pure JWT with no blocklist.  Rather than inventing revocation
// infrastructure here, the residual exposure is bounded by JWT_ACCESS_TTL
// (default 15m) — the access token simply expires and can never be refreshed.
func (s *Service) deprovision(ctx context.Context, evt UserChanged) (Outcome, error) {
	u, err := s.resolve(ctx, evt)
	if errors.Is(err, userdom.ErrNotFound) {
		// Never provisioned locally, or already deactivated — both are
		// converged states; the push is acknowledged so identity does not
		// log a delivery failure for a benign no-op.
		return OutcomeNotProvisioned, nil
	}
	if err != nil {
		return "", err
	}
	if u.IsService {
		// Service/bridge accounts (ADR-038, users.is_service) are Paca
		// infrastructure, not Vortex-mirrored humans — never let an email
		// coincidence deprovision one.
		s.log.Warn("nexussync: ignoring user.changed for service account",
			"user_id", u.ID, "username", u.Username, "vortex_user_id", evt.UserID)
		return OutcomeNoChange, nil
	}
	if err := s.users.Delete(ctx, u.ID); err != nil {
		return "", fmt.Errorf("nexussync: deprovision user %s: %w", u.ID, err)
	}
	s.log.Info("nexussync: deprovisioned user",
		"user_id", u.ID, "username", u.Username,
		"vortex_user_id", evt.UserID, "vortex_status", evt.Status, "vortex_deleted", evt.Deleted)
	return OutcomeDeprovisioned, nil
}

// restore reactivates a previously deprovisioned user when Vortex reports the
// account active again.
//
// Restore is deliberately stricter than deprovision: only an exact oidc_sub
// link is honored, never the email fallback.  Paca cannot distinguish
// "deactivated by this webhook" from "deleted by a Paca admin" — deleted_at
// is the one flag both paths share — so resurrecting an unlinked account on a
// mere email match could silently undo a deliberate local delete.  For
// oidc_sub-linked accounts Vortex is authoritative for lifecycle (ADR-040
// §2), so clearing deleted_at is correct even if the row was deleted locally
// in the meantime; a Paca admin who wants such a user gone must disable them
// in Vortex.
func (s *Service) restore(ctx context.Context, evt UserChanged) (Outcome, error) {
	u, err := s.users.FindByOIDCSubIncludingDeleted(ctx, evt.UserID)
	if errors.Is(err, userdom.ErrNotFound) {
		// Nothing mirrored for this subject; the user will be JIT-provisioned
		// on their next SSO login (galaxyauth.ResolveOIDCUser).
		return OutcomeNotProvisioned, nil
	}
	if err != nil {
		return "", err
	}
	if u.DeletedAt == nil {
		return OutcomeNoChange, nil
	}
	if err := s.users.Restore(ctx, u.ID); err != nil {
		return "", fmt.Errorf("nexussync: restore user %s: %w", u.ID, err)
	}
	s.log.Info("nexussync: restored user",
		"user_id", u.ID, "username", u.Username, "vortex_user_id", evt.UserID)
	return OutcomeRestored, nil
}

// resolve maps the event to a local active user: the oidc_sub link wins
// (users.oidc_sub stores the Vortex user id); an active account with the
// event's email is the fallback so users who were mirrored before being
// SSO-linked are still deprovisioned.
func (s *Service) resolve(ctx context.Context, evt UserChanged) (*userdom.User, error) {
	u, err := s.users.FindByOIDCSub(ctx, evt.UserID)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, userdom.ErrNotFound) {
		return nil, err
	}
	if evt.Email == "" {
		return nil, userdom.ErrNotFound
	}
	return s.users.FindByEmail(ctx, evt.Email)
}
