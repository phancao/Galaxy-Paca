package galaxyauth

// Bearer authentication against the trusted Vortex issuer (ADR-038):
// RS256 tokens carry the platform identity; the effective principal is the
// act_as claim (an agent acting on a user's behalf) falling back to the
// token's own sub.  Principals map to local users via users.oidc_sub, and
// act_as_agent is recorded for attribution only, granting nothing beyond the
// principal user's permissions.
//
// An unknown principal is rejected — unless delegated JIT provisioning is
// configured (WithDelegatedJIT, SPA-6550) AND every one of its gates passes:
// the token is a delegation presented by a configured trusted delegator, it
// names this deployment's tenant, the person has no locally deprovisioned
// account, and identity confirms — live, fail-closed — that the person is
// active and a member of this tenant. Only then is an account created, with
// the lowest built-in role. Signature, issuer, audience and scope checks run
// first and are untouched.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	userdom "github.com/Paca-AI/api/internal/domain/user"
	"github.com/Paca-AI/api/internal/platform/vortexidentity"
)

// TokenVerifier validates a raw token against the trusted issuer (signature
// via JWKS, iss, exp, alg=RS256).  Implemented by *oidc.Provider.
type TokenVerifier interface {
	VerifyToken(ctx context.Context, rawToken, expectedAudience string) (jwt.MapClaims, error)
}

// BearerUserStore is the subset of UserStore the bearer path needs.
type BearerUserStore interface {
	FindByOIDCSub(ctx context.Context, sub string) (*userdom.User, error)
}

// DirectoryLookup asks the Vortex identity service about a user right now.
// Implemented by *vortexidentity.Client.
type DirectoryLookup interface {
	LookupUser(ctx context.Context, userID string) (*vortexidentity.User, error)
}

// DelegatedProvisioner writes the local account for a delegated principal.
// Implemented by *Service (ProvisionDelegatedUser).
type DelegatedProvisioner interface {
	ProvisionDelegatedUser(ctx context.Context, id Identity) (*userdom.User, error)
}

// DeletedUserFinder finds a user by OIDC subject including soft-deleted rows.
// Implemented by the postgres UserRepository.
type DeletedUserFinder interface {
	FindByOIDCSubIncludingDeleted(ctx context.Context, sub string) (*userdom.User, error)
}

// DelegatedJIT configures just-in-time account creation on the bearer path
// (SPA-6550). Every field is required; a partially filled config leaves JIT
// off rather than running with a gate missing.
type DelegatedJIT struct {
	// Tenant is the tenant code this authenticator's graph serves. The token's
	// effective tenant (act_as_tenant, else tenant) must equal it, and
	// identity must list the person as an active member of it.
	Tenant string
	// Delegators are the token subjects / actors trusted to present a token on
	// a person's behalf for the purpose of creating their account (e.g. the
	// aggregating MCP gateway "galaxy-gateway").
	Delegators []string
	// Directory confirms the person with identity (status + memberships).
	Directory DirectoryLookup
	// Provisioner creates the account.
	Provisioner DelegatedProvisioner
	// Deleted detects a locally deprovisioned account for the same subject.
	Deleted DeletedUserFinder
}

func (j *DelegatedJIT) complete() bool {
	return j != nil && strings.TrimSpace(j.Tenant) != "" && len(j.Delegators) > 0 &&
		j.Directory != nil && j.Provisioner != nil && j.Deleted != nil
}

func (j *DelegatedJIT) trusts(actor string) bool {
	for _, d := range j.Delegators {
		if d == actor {
			return true
		}
	}
	return false
}

// BearerAuthenticator resolves trusted-issuer bearer tokens to local users.
// It satisfies the transport middleware's GalaxyBearerAuthenticator contract.
type BearerAuthenticator struct {
	verifier TokenVerifier
	users    BearerUserStore
	log      *slog.Logger

	// jit, when non-nil, enables delegated JIT account creation (SPA-6550).
	jit *DelegatedJIT

	// audience, when non-empty, is enforced as the token's aud claim (PACA-C1):
	// a token minted for a different resource is rejected, closing the
	// confused-deputy hole where a bearer token issued for another audience
	// could be replayed against Paca.
	audience string

	// resourceScopePrefix, when non-empty, enables scope enforcement (PACA-C1).
	// It is Paca's own resource-scope prefix (e.g. "mcp:paca:"). A token that
	// carries any resource-family scope (the segment up to the first colon,
	// e.g. "mcp:") but none for Paca is rejected as a foreign-resource token,
	// and a token whose Paca scopes are read-only is denied write operations.
	resourceScopePrefix string

	// fleetScopePrefix names the FLEET scope that reaches every MCP server
	// through one connector (Vortex ADR-043: one endpoint, one login, no
	// per-server OAuth storm). A token carrying it is not a foreign-resource
	// token — it is the designed way an aggregating gateway calls Paca on a
	// user's behalf, and Paca is one of the servers it aggregates.
	//
	// Without this, PACA-C1's foreign-resource rule fires on exactly the
	// intended path: the gateway forwards the caller's ORIGINAL token
	// (paca_client.py: "forwarded unchanged"), that token's scopes are
	// `mcp:galaxy:read`/`:write`, they share the `mcp:` family but none has
	// the `mcp:paca:` prefix — so every call 401s with "scope does not grant
	// access to this resource". Measured on SpaxeAI 24/09/2026.
	//
	// Read/write still bites: `mcp:galaxy:read` alone denies write methods,
	// because scopeAction reads the segment after the final colon either way.
	// A genuinely foreign token (`mcp:wiki:read`) is still refused.
	fleetScopePrefix string

	// legacyScopePrefixes lists prefixes this service answered to under a
	// FORMER NAME. Paca was called `pm`, and the rename reached every tool
	// name a client sees (`pm__*` is gone, deliberately) but never reached
	// the scope catalogue: identity still issues `mcp:pm:read`/`:write` and
	// has no `mcp:paca:*` row at all. So PACA-C1 read its own old name as a
	// foreign resource and refused every call.
	//
	// This is not a name→permission bridge (the kind PACA-3/4 removed). It is
	// the same shape as reading both `NEXUS_` and `VORTEX_` for one secret:
	// two spellings, one thing. Read/write still comes from the scope's own
	// action segment, so `mcp:pm:read` grants exactly reads.
	legacyScopePrefixes []string
}

// NewBearerAuthenticator returns a configured BearerAuthenticator.
func NewBearerAuthenticator(verifier TokenVerifier, users BearerUserStore, log *slog.Logger) *BearerAuthenticator {
	return &BearerAuthenticator{verifier: verifier, users: users, log: log}
}

// WithResourceAudience configures the aud value enforced on every bearer token
// (PACA-C1). Empty leaves audience enforcement off. Returns the receiver for
// fluent wiring.
func (a *BearerAuthenticator) WithResourceAudience(aud string) *BearerAuthenticator {
	a.audience = strings.TrimSpace(aud)
	return a
}

// WithResourceScopePrefix configures Paca's own resource-scope prefix (e.g.
// "mcp:paca:") used for scope enforcement (PACA-C1). Empty disables scope
// checks. Returns the receiver for fluent wiring.
func (a *BearerAuthenticator) WithResourceScopePrefix(prefix string) *BearerAuthenticator {
	a.resourceScopePrefix = strings.TrimSpace(prefix)
	return a
}

// WithFleetScopePrefix configures the fleet scope prefix that an aggregating
// MCP gateway carries (e.g. "mcp:galaxy:"). Empty leaves fleet scopes treated
// as foreign. Returns the receiver for fluent wiring.
func (a *BearerAuthenticator) WithFleetScopePrefix(prefix string) *BearerAuthenticator {
	a.fleetScopePrefix = strings.TrimSpace(prefix)
	return a
}

// WithLegacyScopePrefixes configures prefixes this service answered to under a
// former name (e.g. "mcp:pm:"). Empty entries are dropped.
func (a *BearerAuthenticator) WithLegacyScopePrefixes(prefixes []string) *BearerAuthenticator {
	a.legacyScopePrefixes = nil
	for _, p := range prefixes {
		if p = strings.TrimSpace(p); p != "" {
			a.legacyScopePrefixes = append(a.legacyScopePrefixes, p)
		}
	}
	return a
}

// WithDelegatedJIT enables just-in-time account creation for delegated bearer
// tokens (SPA-6550). An incomplete config leaves it disabled. Returns the
// receiver for fluent wiring.
func (a *BearerAuthenticator) WithDelegatedJIT(cfg DelegatedJIT) *BearerAuthenticator {
	cfg.Tenant = strings.ToLower(strings.TrimSpace(cfg.Tenant))
	var delegators []string
	for _, d := range cfg.Delegators {
		if d = strings.TrimSpace(d); d != "" {
			delegators = append(delegators, d)
		}
	}
	cfg.Delegators = delegators
	if !cfg.complete() {
		a.jit = nil
		return a
	}
	a.jit = &cfg
	return a
}

// DelegatedJITEnabled reports whether delegated JIT provisioning is active.
func (a *BearerAuthenticator) DelegatedJITEnabled() bool {
	return a.jit != nil
}

// grantsPaca reports whether s is one of this service's own scopes, under any
// name it has ever had, or the fleet scope that reaches it.
func (a *BearerAuthenticator) grantsPaca(s string) bool {
	if strings.HasPrefix(s, a.resourceScopePrefix) {
		return true
	}
	if a.fleetScopePrefix != "" && strings.HasPrefix(s, a.fleetScopePrefix) {
		return true
	}
	for _, p := range a.legacyScopePrefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// AuthenticateBearer verifies rawToken and returns the effective local user
// plus the attributed agent name (empty when the call is not agent-attributed).
//
// method is the HTTP method of the request being authorized; it is used only
// for scope enforcement (write methods require a non-read-only Paca scope when
// the token carries scopes at all). Pass an empty string for method-agnostic
// callers, which are then treated as reads.
func (a *BearerAuthenticator) AuthenticateBearer(ctx context.Context, rawToken, method string) (*userdom.User, string, error) {
	claims, err := a.verifier.VerifyToken(ctx, rawToken, a.audience)
	if err != nil {
		return nil, "", err
	}

	if err := a.enforceScope(claims, method); err != nil {
		return nil, "", err
	}

	tokenSub, _ := claims["sub"].(string)
	principal := effectivePrincipalSub(claims, tokenSub)
	if principal == "" {
		return nil, "", fmt.Errorf("galaxyauth: bearer token has no usable subject")
	}

	u, err := a.users.FindByOIDCSub(ctx, principal)
	if errors.Is(err, userdom.ErrNotFound) && a.jit != nil {
		// Unknown principal: only a delegation that passes every JIT gate
		// gets an account; anything else is rejected exactly as before.
		u, err = a.provisionDelegated(ctx, claims, tokenSub, principal)
	}
	if err != nil {
		return nil, "", fmt.Errorf("galaxyauth: resolve bearer principal: %w", err)
	}

	agentName := agentAttribution(claims)
	if agentName != "" {
		a.log.Info("galaxyauth: agent-attributed bearer call",
			"agent", agentName, "principal_user", u.Username, "token_sub", tokenSub)
	}
	return u, agentName, nil
}

// provisionDelegated runs the delegated-JIT gates for an unknown principal and,
// when all pass, creates the account (SPA-6550). Every refusal keeps
// userdom.ErrNotFound in the chain so callers and logs read the same as the
// pre-JIT rejection, with the reason appended.
func (a *BearerAuthenticator) provisionDelegated(ctx context.Context, claims jwt.MapClaims, tokenSub, principal string) (*userdom.User, error) {
	refuse := func(reason string, args ...any) (*userdom.User, error) {
		why := fmt.Sprintf(reason, args...)
		a.log.Warn("galaxyauth: bearer JIT refused",
			"principal", principal, "token_sub", tokenSub, "reason", why)
		return nil, fmt.Errorf("%w (no account created: %s)", userdom.ErrNotFound, why)
	}

	actor := delegatingActor(claims, tokenSub, principal)
	if actor == "" {
		return refuse("token is not a delegation")
	}
	if !a.jit.trusts(actor) {
		return refuse("delegator %q is not trusted", actor)
	}
	tenant := tokenTenant(claims)
	if tenant != a.jit.Tenant {
		return refuse("token tenant %q is not this deployment's tenant %q", tenant, a.jit.Tenant)
	}

	prev, err := a.jit.Deleted.FindByOIDCSubIncludingDeleted(ctx, principal)
	switch {
	case err == nil && prev.DeletedAt != nil:
		// Deprovisioned here (identity sync or a Paca admin). Re-creating
		// a fresh account would silently undo that; restoring is
		// nexussync's job, with its own confirmation.
		return refuse("the local account for this person is deactivated")
	case err == nil:
		// An active row appeared between the two reads (concurrent JIT).
		return prev, nil
	case !errors.Is(err, userdom.ErrNotFound):
		return nil, fmt.Errorf("galaxyauth: bearer JIT: check deactivated account: %w", err)
	}

	person, err := a.jit.Directory.LookupUser(ctx, principal)
	if err != nil {
		// Fail closed: identity unreachable, unconfigured, or unaware of
		// the person — no account.
		return refuse("identity could not confirm the person: %v", err)
	}
	if !person.Active() {
		return refuse("identity lists the person as %q, not active", person.Status)
	}
	if !person.MemberOf(a.jit.Tenant) {
		return refuse("identity lists no active membership in tenant %q", a.jit.Tenant)
	}

	u, err := a.jit.Provisioner.ProvisionDelegatedUser(ctx, Identity{
		Subject: principal,
		Email:   person.Email,
		Name:    person.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("galaxyauth: bearer JIT: %w", err)
	}
	a.log.Info("galaxyauth: bearer JIT provisioned delegated principal",
		"user_id", u.ID, "username", u.Username, "delegator", actor, "tenant", a.jit.Tenant)
	return u, nil
}

// delegatingActor returns who presented the token on the principal's behalf,
// or "" when the token is not a delegation:
//   - flat act_as (the AI-proxy contract): the token's own sub asserted it;
//   - RFC 8693 impersonation (sub = person, act.sub = the delegating client,
//     e.g. "galaxy-gateway"): the OUTERMOST act.sub — the current actor; any
//     prior chain nested inside it is provenance, not authority.
func delegatingActor(claims jwt.MapClaims, tokenSub, principal string) string {
	if principal != tokenSub {
		// The principal came from act_as, so the token's subject is the one
		// acting for it.
		return tokenSub
	}
	if act, ok := claims["act"].(map[string]any); ok {
		if s, ok := act["sub"].(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// tokenTenant mirrors the OIDC handler's effectiveTenant (ADR-058):
// act_as_tenant, else tenant, trimmed and lower-cased.
func tokenTenant(claims jwt.MapClaims) string {
	for _, key := range []string{"act_as_tenant", "tenant"} {
		if v, ok := claims[key].(string); ok {
			if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
				return v
			}
		}
	}
	return ""
}

// effectivePrincipalSub extracts the acted-for subject from the act_as claim,
// accepting both the object form {"sub": "..."} and a plain string, and
// falling back to the token's own sub.  Malformed act_as values fall back
// rather than fail so a bare platform token still authenticates as itself.
func effectivePrincipalSub(claims jwt.MapClaims, tokenSub string) string {
	actAs, ok := claims["act_as"]
	if !ok || actAs == nil {
		return tokenSub
	}
	switch v := actAs.(type) {
	case string:
		if v != "" {
			return v
		}
	case map[string]any:
		if s, ok := v["sub"].(string); ok && s != "" {
			return s
		}
	}
	return tokenSub
}

// enforceScope applies OAuth-scope enforcement to a verified token (PACA-C1).
// It is a no-op when scope enforcement is disabled (no resource-scope prefix
// configured) or when the token carries no scope claim at all — the audience
// check remains the primary defense in those cases. When the token DOES carry
// resource-family scopes, it must include at least one Paca scope (else it is a
// foreign-resource token) and, for write methods, at least one non-read-only
// Paca scope.
func (a *BearerAuthenticator) enforceScope(claims jwt.MapClaims, method string) error {
	if a.resourceScopePrefix == "" {
		return nil
	}
	scopes := scopeList(claims)
	if len(scopes) == 0 {
		// No scope claim present: nothing to gate on. (Service/act_as tokens
		// minted by identity carry no scope.)
		return nil
	}

	family := scopeFamily(a.resourceScopePrefix)
	var resourceScopes, pacaScopes []string
	for _, s := range scopes {
		if family != "" && strings.HasPrefix(s, family) {
			resourceScopes = append(resourceScopes, s)
		}
		if a.grantsPaca(s) {
			pacaScopes = append(pacaScopes, s)
		}
	}

	// The token declares no resource-family scopes at all (only generic scopes
	// like "openid profile"): don't gate on resource scope.
	if len(resourceScopes) == 0 {
		return nil
	}

	// Resource-scoped token that targets other resources but not Paca — a
	// confused-deputy / foreign-resource token.
	if len(pacaScopes) == 0 {
		// Nêu ĐÍCH DANH scope nào — một câu từ chối không nói nó thấy gì
		// buộc người vận hành đi giải mã token bằng tay để biết phải sửa
		// cái gì. Scope là tên năng lực, không phải bí mật.
		return fmt.Errorf("galaxyauth: bearer token scope does not grant access to this resource "+
			"(token carries %v; need prefix %q, fleet prefix %q, or a legacy prefix %v)",
			resourceScopes, a.resourceScopePrefix, a.fleetScopePrefix, a.legacyScopePrefixes)
	}

	if isWriteMethod(method) && isReadOnlyScopes(pacaScopes) {
		return fmt.Errorf("galaxyauth: bearer token is read-only; %s requires a write scope", method)
	}
	return nil
}

// scopeList extracts the OAuth scopes from a token's scope claim, accepting the
// space-delimited string form ("scope"/"scp") and the JSON array forms.
func scopeList(claims jwt.MapClaims) []string {
	for _, key := range []string{"scope", "scp"} {
		raw, ok := claims[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case string:
			return strings.Fields(v)
		case []string:
			return v
		case []any:
			out := make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, s)
				}
			}
			return out
		}
	}
	return nil
}

// scopeFamily returns the resource family of a scope prefix: the segment up to
// and including the first colon (e.g. "mcp:paca:" -> "mcp:"). A prefix with no
// colon is its own family.
func scopeFamily(prefix string) string {
	if i := strings.Index(prefix, ":"); i >= 0 {
		return prefix[:i+1]
	}
	return prefix
}

// isReadOnlyScopes reports whether every Paca scope grants only read access —
// i.e. its action (the segment after the final colon) is exactly "read".
func isReadOnlyScopes(pacaScopes []string) bool {
	for _, s := range pacaScopes {
		if scopeAction(s) != "read" {
			return false
		}
	}
	return true
}

// scopeAction returns the action segment of a scope (the part after the final
// colon), e.g. "mcp:paca:write" -> "write".
func scopeAction(scope string) string {
	if i := strings.LastIndex(scope, ":"); i >= 0 {
		return scope[i+1:]
	}
	return scope
}

// isWriteMethod reports whether an HTTP method mutates state.
func isWriteMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// agentAttribution extracts a display name for the acting agent from the
// act_as_agent claim (string, or an object exposing name/handle/sub/id).
func agentAttribution(claims jwt.MapClaims) string {
	raw, ok := claims["act_as_agent"]
	if !ok || raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return v
	case map[string]any:
		for _, key := range []string{"name", "handle", "sub", "id"} {
			if s, ok := v[key].(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}
