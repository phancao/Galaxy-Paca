// Package jwttoken signs and verifies HS256 JWTs using golang-jwt/jwt.
package jwttoken

import (
	"fmt"
	"time"

	domainauth "github.com/Paca-AI/api/internal/domain/auth"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Manager handles JWT creation and verification.
type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	tenant     string
	// legacy is true only for the tenant that already held this deployment's
	// data — the one whose users are carrying tokens minted before the
	// tenant claim existed. Only it may accept a token that names no tenant.
	legacy bool
}

// New returns a Manager configured with the given secret and token lifetimes.
func New(secret string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

// ForTenant returns a copy of the Manager that stamps `tenant` on every token
// it signs and REFUSES every token naming a different one.
//
// The tenant lives on the Manager rather than in each Issue… call because a
// tenant is not something a caller should be able to choose: there is exactly
// one right answer per dependency graph, and threading it through twenty call
// sites is twenty chances to pass the wrong one. Here it cannot be passed at
// all, only configured once, next to the secret it is verified with.
//
// One process, one signing secret, several tenants — so refusing on the way
// IN is what keeps a session from crossing over. Signing alone would not:
// every tenant's tokens verify against the same key.
//
// `acceptsLegacy` must be true for exactly ONE tenant: the primary, whose
// users hold tokens minted before the claim existed. Set it anywhere else and
// an old session would be admitted by every workspace in the process.
func (m *Manager) ForTenant(tenant string, acceptsLegacy bool) *Manager {
	c := *m
	c.tenant = tenant
	c.legacy = acceptsLegacy
	return &c
}

// Tenant reports which tenant this Manager signs and accepts tokens for.
func (m *Manager) Tenant() string { return m.tenant }

// IssueAccess creates a signed access token for the given claims subject.
func (m *Manager) IssueAccess(sub, username, role, familyID string, mustChangePassword bool) (string, error) {
	return m.sign(sub, username, role, familyID, m.accessTTL, "access", false, mustChangePassword)
}

// IssueRefresh creates a signed refresh token with the Manager's default TTL.
// The session is treated as persistent (rememberMe=true).
func (m *Manager) IssueRefresh(sub, username, role, familyID string) (string, error) {
	return m.sign(sub, username, role, familyID, m.refreshTTL, "refresh", true, false)
}

// IssueRefreshWithTTL creates a signed refresh token with an explicit TTL and
// rememberMe flag. Use this instead of IssueRefresh when the caller needs to
// honour the user's "remember me" preference.
func (m *Manager) IssueRefreshWithTTL(sub, username, role, familyID string, rememberMe bool, ttl time.Duration) (string, error) {
	return m.sign(sub, username, role, familyID, ttl, "refresh", rememberMe, false)
}

func (m *Manager) sign(sub, username, role, familyID string, ttl time.Duration, kind string, rememberMe bool, mustChangePassword bool) (string, error) {
	now := time.Now()
	claims := domainauth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		Username:           username,
		Role:               role,
		Kind:               kind,
		FamilyID:           familyID,
		RememberMe:         rememberMe,
		MustChangePassword: mustChangePassword,
		Tenant:             m.tenant,
	}

	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("token: sign: %w", err)
	}
	return signed, nil
}

// Verify parses and validates a token, returning its claims.
func (m *Manager) Verify(tokenStr string) (*domainauth.Claims, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &domainauth.Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("token: unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("token: verify: %w", err)
	}

	claims, ok := t.Claims.(*domainauth.Claims)
	if !ok || !t.Valid {
		return nil, fmt.Errorf("token: invalid claims")
	}

	// A valid signature says the token is ours; it does not say it is THIS
	// tenant's. All tenants in a process share one secret, so a session from
	// another workspace verifies perfectly and would otherwise be admitted
	// into a database it has no business in.
	//
	// A token naming NO tenant predates this claim. Those were all issued by
	// the tenant that is now primary, so only the primary accepts them —
	// letting every tenant accept them would turn a compatibility allowance
	// into a skeleton key.
	if claims.Tenant == "" {
		// A Manager that was never given a tenant is a single-tenant
		// deployment: it signs no claim and accepts none, exactly as before
		// any of this existed.
		if m.tenant != "" && !m.legacy {
			return nil, fmt.Errorf("token: names no tenant, and %q is not the primary", m.tenant)
		}
	} else if claims.Tenant != m.tenant {
		return nil, fmt.Errorf("token: minted for tenant %q, presented to %q", claims.Tenant, m.tenant)
	}

	return claims, nil
}
