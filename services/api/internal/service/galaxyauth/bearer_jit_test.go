package galaxyauth

// SPA-6550 (Bugbase 2cdba544): a person reaching Paca through the aggregating
// MCP gateway before ever opening the web app got 401 "user: not found" on
// every paca_* tool. These tests pin the delegated-JIT gates: the account is
// created ONLY for a delegation from a trusted delegator, naming this tenant,
// for a person identity confirms active and a member of this tenant — and
// every other case stays rejected without a row being written.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	globalroledom "github.com/Paca-AI/api/internal/domain/globalrole"
	userdom "github.com/Paca-AI/api/internal/domain/user"
	"github.com/Paca-AI/api/internal/platform/oidc"
	"github.com/Paca-AI/api/internal/platform/vortexidentity"
)

const (
	jitTenant   = "spaxe"
	jitPerson   = "b324afac-8ab6-4d50-8f5b-e86bc5f068d4"
	jitGateway  = "galaxy-gateway"
	jitAudience = "https://tasks.example/paca"
)

// fakeDirectory stands in for identity's /internal/users.
type fakeDirectory struct {
	mu    sync.Mutex
	users map[string]*vortexidentity.User
	err   error
	calls atomic.Int32
}

func (f *fakeDirectory) LookupUser(_ context.Context, id string) (*vortexidentity.User, error) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	if u, ok := f.users[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, vortexidentity.ErrUnknownUser
}

func activeMember() *fakeDirectory {
	return &fakeDirectory{users: map[string]*vortexidentity.User{
		jitPerson: {ID: jitPerson, Email: "cao.phan@spaxeai.com", Name: "Cao Phan", Status: "active", Tenants: []string{"spaxe"}},
	}}
}

type jitFixture struct {
	auth  *BearerAuthenticator
	store *memUserStore
	dir   *fakeDirectory
	sign  func(jwt.MapClaims) string
}

func newJITFixture(t *testing.T, dir *fakeDirectory) *jitFixture {
	t.Helper()
	issuer, key, kid := newFakeJWKSIssuer(t)
	store := newMemUserStore()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	roles := &memRoleFinder{roles: map[string]*globalroledom.GlobalRole{
		"USER":  {ID: uuid.New(), Name: "USER", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		"ADMIN": {ID: uuid.New(), Name: "ADMIN", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}}
	// OIDC default role deliberately ADMIN: the delegated path must ignore it.
	svc := New(store, roles, true, "ADMIN", log)
	auth := NewBearerAuthenticator(oidc.NewProvider(issuer), store, log).
		WithResourceAudience(jitAudience).
		WithResourceScopePrefix("mcp:paca:").
		WithFleetScopePrefix("mcp:galaxy:").
		WithDelegatedJIT(DelegatedJIT{
			Tenant:      jitTenant,
			Delegators:  []string{jitGateway},
			Directory:   dir,
			Provisioner: svc,
			Deleted:     store,
		})
	if !auth.DelegatedJITEnabled() {
		t.Fatal("fixture: delegated JIT should be enabled")
	}
	return &jitFixture{auth: auth, store: store, dir: dir, sign: func(c jwt.MapClaims) string {
		if _, ok := c["aud"]; !ok {
			c["aud"] = jitAudience
		}
		return signBearer(t, issuer, key, kid, c)
	}}
}

// gatewayClaims is the shape identity mints for the MCP gateway's RFC 8693
// impersonation hop: sub = the person, act.sub = galaxy-gateway.
func gatewayClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"sub":    jitPerson,
		"act":    map[string]any{"sub": jitGateway},
		"tenant": jitTenant,
		"scope":  "mcp:galaxy:read mcp:galaxy:write",
	}
}

func assertRejectedWithoutRow(t *testing.T, f *jitFixture, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected the bearer call to be rejected")
	}
	if n := f.store.count(); n != 0 {
		t.Fatalf("no account may be created on a refused call, store has %d", n)
	}
}

func TestBearerJITProvisionsGatewayDelegatedPerson(t *testing.T) {
	f := newJITFixture(t, activeMember())

	u, _, err := f.auth.AuthenticateBearer(context.Background(), f.sign(gatewayClaims()), http.MethodGet)
	if err != nil {
		t.Fatalf("delegated call by an active tenant member must succeed, got %v", err)
	}
	if u.OIDCSub != jitPerson {
		t.Fatalf("account must be linked to the Vortex user id, got oidc_sub %q", u.OIDCSub)
	}
	if u.Role != userdom.RoleUser {
		t.Fatalf("delegated JIT must use the lowest role USER (not OIDC_DEFAULT_ROLE), got %q", u.Role)
	}
	if u.Email != "cao.phan@spaxeai.com" || u.FullName != "Cao Phan" {
		t.Fatalf("email/name must come from identity, got %q / %q", u.Email, u.FullName)
	}
	if u.Username != "cao.phan" {
		t.Fatalf("username must derive from the identity email, got %q", u.Username)
	}

	// Second call: resolved from the table, identity not asked again.
	u2, _, err := f.auth.AuthenticateBearer(context.Background(), f.sign(gatewayClaims()), http.MethodPost)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if u2.ID != u.ID {
		t.Fatal("second call must resolve the same account")
	}
	if got := f.dir.calls.Load(); got != 1 {
		t.Fatalf("identity must be asked once (only when the account is missing), asked %d times", got)
	}
	if f.store.count() != 1 {
		t.Fatalf("exactly one account expected, got %d", f.store.count())
	}
}

func TestBearerJITProvisionsFlatActAsFromTrustedService(t *testing.T) {
	f := newJITFixture(t, activeMember())

	u, _, err := f.auth.AuthenticateBearer(context.Background(), f.sign(jwt.MapClaims{
		"sub":           jitGateway,
		"act_as":        map[string]any{"sub": jitPerson},
		"act_as_tenant": jitTenant,
	}), http.MethodGet)
	if err != nil {
		t.Fatalf("flat act_as from a trusted delegator must provision, got %v", err)
	}
	if u.OIDCSub != jitPerson {
		t.Fatalf("principal must be the act_as subject, got %q", u.OIDCSub)
	}
}

func TestBearerJITRejectsInactivePerson(t *testing.T) {
	dir := activeMember()
	dir.users[jitPerson].Status = "suspended"
	f := newJITFixture(t, dir)

	_, _, err := f.auth.AuthenticateBearer(context.Background(), f.sign(gatewayClaims()), http.MethodGet)
	assertRejectedWithoutRow(t, f, err)
	if !errors.Is(err, userdom.ErrNotFound) {
		t.Fatalf("refusal must keep the not-found cause, got %v", err)
	}
}

func TestBearerJITRejectsPersonWithoutMembershipInThisTenant(t *testing.T) {
	dir := activeMember()
	dir.users[jitPerson].Tenants = []string{"galaxy", "tmo"}
	f := newJITFixture(t, dir)

	_, _, err := f.auth.AuthenticateBearer(context.Background(), f.sign(gatewayClaims()), http.MethodGet)
	assertRejectedWithoutRow(t, f, err)
}

func TestBearerJITFailsClosedWhenIdentityUnreachable(t *testing.T) {
	dir := activeMember()
	dir.err = errors.New("dial tcp vortex-identity:8086: connection refused")
	f := newJITFixture(t, dir)

	_, _, err := f.auth.AuthenticateBearer(context.Background(), f.sign(gatewayClaims()), http.MethodGet)
	assertRejectedWithoutRow(t, f, err)
}

func TestBearerJITFailsClosedWhenIdentityUnconfigured(t *testing.T) {
	dir := activeMember()
	dir.err = vortexidentity.ErrNotConfigured
	f := newJITFixture(t, dir)

	_, _, err := f.auth.AuthenticateBearer(context.Background(), f.sign(gatewayClaims()), http.MethodGet)
	assertRejectedWithoutRow(t, f, err)
}

func TestBearerJITFailsClosedWhenIdentityDoesNotKnowPerson(t *testing.T) {
	f := newJITFixture(t, &fakeDirectory{users: map[string]*vortexidentity.User{}})

	_, _, err := f.auth.AuthenticateBearer(context.Background(), f.sign(gatewayClaims()), http.MethodGet)
	assertRejectedWithoutRow(t, f, err)
}

func TestBearerJITRejectsTokenWithoutDelegation(t *testing.T) {
	f := newJITFixture(t, activeMember())

	// The person's own token, no act / act_as: not a delegation.
	_, _, err := f.auth.AuthenticateBearer(context.Background(), f.sign(jwt.MapClaims{
		"sub":    jitPerson,
		"tenant": jitTenant,
	}), http.MethodGet)
	assertRejectedWithoutRow(t, f, err)
	if f.dir.calls.Load() != 0 {
		t.Fatal("identity must not be asked for a non-delegated token")
	}
}

func TestBearerJITRejectsUntrustedDelegator(t *testing.T) {
	f := newJITFixture(t, activeMember())

	claims := gatewayClaims()
	claims["act"] = map[string]any{"sub": "some-other-client", "act": map[string]any{"sub": jitGateway}}
	_, _, err := f.auth.AuthenticateBearer(context.Background(), f.sign(claims), http.MethodGet)
	assertRejectedWithoutRow(t, f, err)

	// Flat act_as asserted by an untrusted service.
	_, _, err = f.auth.AuthenticateBearer(context.Background(), f.sign(jwt.MapClaims{
		"sub":           "app:random-service",
		"act_as":        jitPerson,
		"act_as_tenant": jitTenant,
	}), http.MethodGet)
	assertRejectedWithoutRow(t, f, err)
	if f.dir.calls.Load() != 0 {
		t.Fatal("identity must not be asked for an untrusted delegator")
	}
}

func TestBearerJITRejectsOtherOrMissingTenant(t *testing.T) {
	f := newJITFixture(t, activeMember())

	other := gatewayClaims()
	other["tenant"] = "galaxy"
	_, _, err := f.auth.AuthenticateBearer(context.Background(), f.sign(other), http.MethodGet)
	assertRejectedWithoutRow(t, f, err)

	none := gatewayClaims()
	delete(none, "tenant")
	_, _, err = f.auth.AuthenticateBearer(context.Background(), f.sign(none), http.MethodGet)
	assertRejectedWithoutRow(t, f, err)
}

func TestBearerJITDoesNotRecreateDeactivatedAccount(t *testing.T) {
	f := newJITFixture(t, activeMember())
	deletedAt := time.Now().Add(-time.Hour)
	old := &userdom.User{ID: uuid.New(), Username: "cao.phan", DeletedAt: &deletedAt}
	f.store.users = append(f.store.users, old)
	f.store.oidcSub[old.ID] = jitPerson

	_, _, err := f.auth.AuthenticateBearer(context.Background(), f.sign(gatewayClaims()), http.MethodGet)
	if err == nil {
		t.Fatal("a locally deactivated person must not get a fresh account")
	}
	if n := f.store.count(); n != 1 {
		t.Fatalf("no new row may be written, store has %d", n)
	}
}

func TestBearerJITRefusesToAdoptAccountHoldingSameEmail(t *testing.T) {
	f := newJITFixture(t, activeMember())
	other := &userdom.User{ID: uuid.New(), Username: "admin"}
	f.store.users = append(f.store.users, other)
	f.store.emails[other.ID] = "cao.phan@spaxeai.com"

	_, _, err := f.auth.AuthenticateBearer(context.Background(), f.sign(gatewayClaims()), http.MethodGet)
	if !errors.Is(err, ErrDelegatedEmailTaken) {
		t.Fatalf("expected ErrDelegatedEmailTaken, got %v", err)
	}
	if f.store.count() != 1 {
		t.Fatal("the existing account must not be duplicated")
	}
	if f.store.oidcSub[other.ID] != "" {
		t.Fatal("the bearer path must never link an existing account by email")
	}
}

func TestBearerJITConcurrentFirstCallsCreateOneAccount(t *testing.T) {
	f := newJITFixture(t, activeMember())
	token := f.sign(gatewayClaims())

	const n = 16
	var wg sync.WaitGroup
	ids := make([]uuid.UUID, n)
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			u, _, err := f.auth.AuthenticateBearer(context.Background(), token, http.MethodGet)
			errs[i] = err
			if u != nil {
				ids[i] = u.ID
			}
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
		if ids[i] != ids[0] {
			t.Fatalf("call %d resolved a different account", i)
		}
	}
	if c := f.store.count(); c != 1 {
		t.Fatalf("concurrent first calls must converge on ONE account, got %d", c)
	}
}

func TestBearerJITDoesNotBypassScopeOrAudience(t *testing.T) {
	f := newJITFixture(t, activeMember())

	foreign := gatewayClaims()
	foreign["scope"] = "mcp:wiki:read"
	_, _, err := f.auth.AuthenticateBearer(context.Background(), f.sign(foreign), http.MethodGet)
	assertRejectedWithoutRow(t, f, err)

	wrongAud := gatewayClaims()
	wrongAud["aud"] = "https://wiki.example/wiki"
	_, _, err = f.auth.AuthenticateBearer(context.Background(), f.sign(wrongAud), http.MethodGet)
	assertRejectedWithoutRow(t, f, err)

	if f.dir.calls.Load() != 0 {
		t.Fatal("identity must not be asked before the token itself is accepted")
	}
}

func TestBearerJITIncompleteConfigStaysOff(t *testing.T) {
	issuer, key, kid := newFakeJWKSIssuer(t)
	store := newMemUserStore()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	dir := activeMember()
	auth := NewBearerAuthenticator(oidc.NewProvider(issuer), store, log).
		WithDelegatedJIT(DelegatedJIT{
			Tenant:     "", // missing tenant: no gate to compare against
			Delegators: []string{jitGateway},
			Directory:  dir,
			Deleted:    store,
		})
	if auth.DelegatedJITEnabled() {
		t.Fatal("an incomplete JIT config must leave JIT off")
	}
	_, _, err := auth.AuthenticateBearer(context.Background(), signBearer(t, issuer, key, kid, gatewayClaims()), http.MethodGet)
	if err == nil || store.count() != 0 || dir.calls.Load() != 0 {
		t.Fatalf("with JIT off an unknown principal must be rejected outright (err=%v rows=%d)", err, store.count())
	}
}
