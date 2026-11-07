package galaxyauth

// Just-in-time provisioning on the bearer path (SPA-6550).
//
// Before this, a Paca account existed only after its owner had signed in to
// the Paca web app once (OIDC SSO → ResolveOIDCUser).  A Vortex user who
// reached Paca only through the MCP gateway — a token with sub=gateway and
// act_as.sub=<Vortex user id> — resolved to no local user and every paca_*
// tool answered 401 AUTH_TOKEN_INVALID, on a brand-new tenant for everyone.
//
// The bearer path now provisions that user, but only under conditions that
// grant nothing the user could not already get by opening Paca once:
//
//   - the token has passed signature/iss/exp/aud/scope checks unchanged;
//   - the principal arrived as act_as (an aggregating gateway acting for the
//     user), never as a token's own sub — a bare service token is not a person;
//   - SSO auto-create is enabled for this deployment (the same switch and the
//     same default global role ResolveOIDCUser uses);
//   - the Vortex identity service itself, asked server-to-server, lists the
//     subject as ACTIVE and as a member of THIS Paca tenant.  Any lookup
//     failure denies (fail closed);
//   - no soft-deleted account is linked to the subject (restoring one stays
//     nexussync's job).
//
// The account is then resolved exactly like an SSO login: an existing account
// with identity's email is linked, otherwise a new one is created with the
// default role.  The email and name come from identity's directory row, not
// from the token.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	userdom "github.com/Paca-AI/api/internal/domain/user"
	"github.com/Paca-AI/api/internal/platform/vortexidentity"
)

// UserDirectory is the Vortex identity read the provisioner trusts.
// Implemented by *vortexidentity.Client.
type UserDirectory interface {
	LookupUser(ctx context.Context, userID string) (*vortexidentity.User, error)
}

// BearerPrincipalProvisioner creates (or links) the local account for an
// act_as principal the bearer path does not know yet.
type BearerPrincipalProvisioner interface {
	ProvisionBearerPrincipal(ctx context.Context, sub string) (*userdom.User, error)
}

// TombstoneFinder finds an oidc_sub-linked account even when it is soft-deleted.
// Implemented by the postgres UserRepository (ADR-040 extensions).
type TombstoneFinder interface {
	FindByOIDCSubIncludingDeleted(ctx context.Context, sub string) (*userdom.User, error)
}

// BearerProvisioner provisions act_as principals confirmed by Vortex identity.
type BearerProvisioner struct {
	svc        *Service
	dir        UserDirectory
	tombstones TombstoneFinder
	tenant     string
	log        *slog.Logger

	// mu serialises provisioning: an MCP client often fires several tool
	// calls at once, and two concurrent creates for one subject would leave
	// an orphan "name-2" account behind the oidc_sub unique index.
	mu sync.Mutex
}

// NewBearerProvisioner returns a provisioner for the Paca tenant tenantCode.
func NewBearerProvisioner(svc *Service, dir UserDirectory, tombstones TombstoneFinder, tenantCode string, log *slog.Logger) *BearerProvisioner {
	return &BearerProvisioner{svc: svc, dir: dir, tombstones: tombstones, tenant: strings.TrimSpace(tenantCode), log: log}
}

// ProvisionBearerPrincipal resolves sub to a local user, creating or linking
// the account when Vortex identity confirms the subject is an active member
// of this tenant.  It returns ErrUserNotProvisioned when any condition fails.
func (p *BearerProvisioner) ProvisionBearerPrincipal(ctx context.Context, sub string) (*userdom.User, error) {
	if p == nil || p.svc == nil || p.dir == nil || p.tombstones == nil || p.tenant == "" || !p.svc.autoCreate || sub == "" {
		return nil, ErrUserNotProvisioned
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// A concurrent call may have provisioned the subject while we waited.
	if u, err := p.svc.users.FindByOIDCSub(ctx, sub); err == nil {
		return u, nil
	} else if !errors.Is(err, userdom.ErrNotFound) {
		return nil, err
	}

	// A soft-deleted account linked to this subject was deprovisioned (by the
	// ADR-040 webhook or a Paca admin).  Bringing it back is nexussync's job,
	// gated by its own identity read-back; an agent call must never mint a
	// fresh account beside it.
	if old, err := p.tombstones.FindByOIDCSubIncludingDeleted(ctx, sub); err == nil && old.DeletedAt != nil {
		p.log.Warn("galaxyauth: bearer JIT declined — account for this subject was deprovisioned",
			"vortex_user_id", sub, "user_id", old.ID, "tenant", p.tenant)
		return nil, fmt.Errorf("%w: account deprovisioned", ErrUserNotProvisioned)
	} else if err != nil && !errors.Is(err, userdom.ErrNotFound) {
		return nil, err
	}

	row, err := p.dir.LookupUser(ctx, sub)
	if err != nil {
		p.log.Warn("galaxyauth: bearer JIT declined — identity lookup failed",
			"vortex_user_id", sub, "tenant", p.tenant, "error", err)
		return nil, fmt.Errorf("%w: identity lookup: %v", ErrUserNotProvisioned, err)
	}
	if !row.Active() {
		p.log.Warn("galaxyauth: bearer JIT declined — user not active in identity",
			"vortex_user_id", sub, "tenant", p.tenant, "status", row.Status)
		return nil, fmt.Errorf("%w: identity status %q", ErrUserNotProvisioned, row.Status)
	}
	if !row.MemberOf(p.tenant) {
		p.log.Warn("galaxyauth: bearer JIT declined — user not a member of this tenant",
			"vortex_user_id", sub, "tenant", p.tenant, "member_of", row.Tenants)
		return nil, fmt.Errorf("%w: not a member of tenant %q", ErrUserNotProvisioned, p.tenant)
	}

	u, err := p.svc.ResolveOIDCUser(ctx, Identity{
		Subject: sub,
		Email:   strings.TrimSpace(row.Email),
		Name:    strings.TrimSpace(row.Name),
	})
	if err != nil {
		return nil, err
	}
	p.log.Info("galaxyauth: bearer principal provisioned from Vortex identity",
		"user_id", u.ID, "username", u.Username, "vortex_user_id", sub, "tenant", p.tenant)
	return u, nil
}
