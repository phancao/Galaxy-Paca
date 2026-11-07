package galaxyauth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	userdom "github.com/Paca-AI/api/internal/domain/user"
	"github.com/Paca-AI/api/internal/platform/oidc"
	"github.com/Paca-AI/api/internal/platform/vortexidentity"
)

// SPA-6550: a Vortex user who reaches Paca only through the MCP gateway
// (sub=gateway, act_as.sub=<Vortex user id>) and never opened the Paca web
// app had no local account, so every paca_* tool answered 401.

const (
	jitTenant     = "spaxe"
	jitVortexUser = "b324afac-8ab6-4d50-8f5b-e86bc5f068d4"
	jitGatewaySub = "galaxy-gateway"
)

// fakeDirectory is an in-memory Vortex identity /internal/users.
type fakeDirectory struct {
	mu    sync.Mutex
	rows  map[string]*vortexidentity.User
	err   error
	calls int
}

func (d *fakeDirectory) LookupUser(_ context.Context, id string) (*vortexidentity.User, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	if d.err != nil {
		return nil, d.err
	}
	if r, ok := d.rows[id]; ok {
		return r, nil
	}
	return nil, vortexidentity.ErrUnknownUser
}

func activeSpaxeMember() *fakeDirectory {
	return &fakeDirectory{rows: map[string]*vortexidentity.User{
		jitVortexUser: {
			ID: jitVortexUser, Email: "cao.phan@spaxeai.com", Name: "Cao Phan",
			Status: "active", Tenants: []string{jitTenant},
		},
	}}
}

// lockedStore makes memUserStore safe for the concurrent test under -race,
// and keeps soft-deleted accounts (tombstones) out of the active lookups.
type lockedStore struct {
	mu         sync.Mutex
	m          *memUserStore
	tombstones map[string]*userdom.User // oidc_sub → soft-deleted account
}

func (s *lockedStore) FindByOIDCSubIncludingDeleted(ctx context.Context, sub string) (*userdom.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.tombstones[sub]; ok {
		return u, nil
	}
	return s.m.FindByOIDCSub(ctx, sub)
}

func (s *lockedStore) FindByOIDCSub(ctx context.Context, sub string) (*userdom.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m.FindByOIDCSub(ctx, sub)
}

func (s *lockedStore) FindByEmail(ctx context.Context, email string) (*userdom.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m.FindByEmail(ctx, email)
}

func (s *lockedStore) FindByUsername(ctx context.Context, username string) (*userdom.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m.FindByUsername(ctx, username)
}

func (s *lockedStore) Create(ctx context.Context, u *userdom.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m.Create(ctx, u)
}

func (s *lockedStore) LinkOIDC(ctx context.Context, id uuid.UUID, sub, email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m.LinkOIDC(ctx, id, sub, email)
}

type jitFixture struct {
	auth   *BearerAuthenticator
	store  *memUserStore
	locked *lockedStore
	dir    *fakeDirectory
	sign   func(jwt.MapClaims) string
}

func newJITFixture(t *testing.T, dir *fakeDirectory, autoCreate bool) *jitFixture {
	t.Helper()
	issuer, key, kid := newFakeJWKSIssuer(t)
	mem := newMemUserStore()
	store := &lockedStore{m: mem, tombstones: map[string]*userdom.User{}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := newTestService(mem, autoCreate)
	svc.users = store
	auth := NewBearerAuthenticator(oidc.NewProvider(issuer), store, log).
		WithResourceScopePrefix("mcp:paca:").
		WithFleetScopePrefix("mcp:galaxy:").
		WithPrincipalProvisioner(NewBearerProvisioner(svc, dir, store, jitTenant, log))
	return &jitFixture{auth: auth, store: mem, locked: store, dir: dir, sign: func(c jwt.MapClaims) string {
		return signBearer(t, issuer, key, kid, c)
	}}
}

func gatewayToken(f *jitFixture) string {
	return f.sign(jwt.MapClaims{
		"sub":    jitGatewaySub,
		"act_as": map[string]any{"sub": jitVortexUser},
		"scope":  "mcp:galaxy:read mcp:galaxy:write",
	})
}

func TestBearerJITProvisionsActAsPrincipalConfirmedByIdentity(t *testing.T) {
	f := newJITFixture(t, activeSpaxeMember(), true)

	u, _, err := f.auth.AuthenticateBearer(context.Background(), gatewayToken(f), http.MethodGet)
	if err != nil {
		t.Fatalf("active tenant member via gateway must be provisioned, got: %v", err)
	}
	if f.store.oidcSub[u.ID] != jitVortexUser {
		t.Fatalf("provisioned user must be linked to the Vortex user id, got %q", f.store.oidcSub[u.ID])
	}
	if f.store.emails[u.ID] != "cao.phan@spaxeai.com" {
		t.Fatalf("email must come from identity's directory row, got %q", f.store.emails[u.ID])
	}
	if u.Role != "USER" || u.Username != "cao.phan" || u.FullName != "Cao Phan" {
		t.Fatalf("unexpected account: role=%q username=%q name=%q", u.Role, u.Username, u.FullName)
	}

	// The next call resolves the same account without asking identity again.
	u2, _, err := f.auth.AuthenticateBearer(context.Background(), gatewayToken(f), http.MethodGet)
	if err != nil || u2.ID != u.ID {
		t.Fatalf("second call must resolve the same user: err=%v", err)
	}
	if len(f.store.users) != 1 || f.dir.calls != 1 {
		t.Fatalf("want 1 user and 1 identity lookup, got %d users, %d lookups", len(f.store.users), f.dir.calls)
	}
}

func TestBearerJITLinksExistingAccountByIdentityEmail(t *testing.T) {
	f := newJITFixture(t, activeSpaxeMember(), true)
	existing := &userdom.User{ID: uuid.New(), Username: "cao", Role: "ADMIN"}
	f.store.users = append(f.store.users, existing)
	f.store.emails[existing.ID] = "cao.phan@spaxeai.com"

	u, _, err := f.auth.AuthenticateBearer(context.Background(), gatewayToken(f), http.MethodGet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.ID != existing.ID || f.store.oidcSub[existing.ID] != jitVortexUser || len(f.store.users) != 1 {
		t.Fatal("an account with identity's email must be linked, not duplicated")
	}
}

func TestBearerJITDeclined(t *testing.T) {
	cases := []struct {
		name       string
		dir        func() *fakeDirectory
		autoCreate bool
		claims     jwt.MapClaims
		wantLookup bool
	}{
		{
			name: "identity reports user inactive",
			dir: func() *fakeDirectory {
				d := activeSpaxeMember()
				d.rows[jitVortexUser].Status = "disabled"
				return d
			},
			autoCreate: true, wantLookup: true,
		},
		{
			name: "user is not a member of this tenant",
			dir: func() *fakeDirectory {
				d := activeSpaxeMember()
				d.rows[jitVortexUser].Tenants = []string{"vtv"}
				return d
			},
			autoCreate: true, wantLookup: true,
		},
		{
			name:       "identity does not list the user",
			dir:        func() *fakeDirectory { return &fakeDirectory{rows: map[string]*vortexidentity.User{}} },
			autoCreate: true, wantLookup: true,
		},
		{
			name:       "identity lookup fails",
			dir:        func() *fakeDirectory { return &fakeDirectory{err: errors.New("connection refused")} },
			autoCreate: true, wantLookup: true,
		},
		{
			name:       "SSO auto-create is off",
			dir:        activeSpaxeMember,
			autoCreate: false, wantLookup: false,
		},
		{
			name:       "principal is the token's own sub, not act_as",
			dir:        activeSpaxeMember,
			autoCreate: true, wantLookup: false,
			claims: jwt.MapClaims{"sub": jitVortexUser},
		},
		{
			name:       "foreign-resource scope is still refused first",
			dir:        activeSpaxeMember,
			autoCreate: true, wantLookup: false,
			claims: jwt.MapClaims{
				"sub": jitGatewaySub, "act_as": map[string]any{"sub": jitVortexUser},
				"scope": "mcp:wiki:read",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newJITFixture(t, tc.dir(), tc.autoCreate)
			token := gatewayToken(f)
			if tc.claims != nil {
				token = f.sign(tc.claims)
			}
			if _, _, err := f.auth.AuthenticateBearer(context.Background(), token, http.MethodGet); err == nil {
				t.Fatal("expected rejection")
			}
			if len(f.store.users) != 0 {
				t.Fatal("no account may be created")
			}
			if (f.dir.calls > 0) != tc.wantLookup {
				t.Fatalf("identity lookups = %d, want lookup=%v", f.dir.calls, tc.wantLookup)
			}
		})
	}
}

func TestBearerJITDoesNotRecreateDeprovisionedAccount(t *testing.T) {
	f := newJITFixture(t, activeSpaxeMember(), true)
	deletedAt := time.Now()
	f.locked.tombstones[jitVortexUser] = &userdom.User{ID: uuid.New(), Username: "cao.phan", DeletedAt: &deletedAt}

	if _, _, err := f.auth.AuthenticateBearer(context.Background(), gatewayToken(f), http.MethodGet); err == nil {
		t.Fatal("a deprovisioned account must not be replaced by a fresh one")
	}
	if len(f.store.users) != 0 || f.dir.calls != 0 {
		t.Fatalf("want no account and no identity lookup, got %d users, %d lookups", len(f.store.users), f.dir.calls)
	}
}

func TestBearerJITConcurrentCallsCreateOneAccount(t *testing.T) {
	f := newJITFixture(t, activeSpaxeMember(), true)
	token := gatewayToken(f)

	var wg sync.WaitGroup
	ids := make([]uuid.UUID, 8)
	errs := make([]error, 8)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			u, _, err := f.auth.AuthenticateBearer(context.Background(), token, http.MethodGet)
			errs[i] = err
			if u != nil {
				ids[i] = u.ID
			}
		}(i)
	}
	wg.Wait()
	for i := range ids {
		if errs[i] != nil || ids[i] != ids[0] {
			t.Fatalf("call %d: err=%v id=%s want %s", i, errs[i], ids[i], ids[0])
		}
	}
	if len(f.store.users) != 1 {
		t.Fatalf("parallel first calls must create exactly one account, got %d", len(f.store.users))
	}
}

func TestBearerWithoutProvisionerStillRejectsUnknownActAs(t *testing.T) {
	auth, store, sign := newBearerFixture(t)
	token := sign(jwt.MapClaims{"sub": jitGatewaySub, "act_as": map[string]any{"sub": jitVortexUser}})
	if _, _, err := auth.AuthenticateBearer(context.Background(), token, http.MethodGet); err == nil {
		t.Fatal("without a provisioner an unknown principal must be rejected")
	}
	if len(store.users) != 0 {
		t.Fatal("no account may be created")
	}
}
