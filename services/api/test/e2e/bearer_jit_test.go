package e2e_test

// SPA-6550: delegated-bearer JIT against REAL Postgres — the partial unique
// indexes on active oidc_sub / email / username (migrations 000016, 000032)
// and mapUserUniqueViolation are what make concurrent first calls converge on
// one account and stop the bearer path from adopting another account's email.
// The in-memory fakes in internal/service/galaxyauth mimic them; this pins the
// real thing.

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	userdom "github.com/Paca-AI/api/internal/domain/user"
	"github.com/Paca-AI/api/internal/platform/oidc"
	"github.com/Paca-AI/api/internal/platform/vortexidentity"
	galaxyauthsvc "github.com/Paca-AI/api/internal/service/galaxyauth"
)

type e2eDirectory struct {
	users map[string]*vortexidentity.User
}

func (d *e2eDirectory) LookupUser(_ context.Context, id string) (*vortexidentity.User, error) {
	if u, ok := d.users[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, vortexidentity.ErrUnknownUser
}

func e2eJWKSIssuer(t *testing.T) (string, func(jwt.MapClaims) string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	const kid = "e2e-jit"
	var issuer string
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "jwks_uri": issuer + "/jwks"})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": kid, "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString([]byte{0x01, 0x00, 0x01}),
		}}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	issuer = srv.URL
	return issuer, func(c jwt.MapClaims) string {
		c["iss"] = issuer
		c["exp"] = time.Now().Add(time.Hour).Unix()
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
		tok.Header["kid"] = kid
		s, err := tok.SignedString(key)
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		return s
	}
}

func TestE2E_BearerJIT_RealPostgres(t *testing.T) {
	env := newE2EEnv(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	issuer, sign := e2eJWKSIssuer(t)

	const person = "b324afac-8ab6-4d50-8f5b-e86bc5f068d4"
	dir := &e2eDirectory{users: map[string]*vortexidentity.User{
		person: {ID: person, Email: "jit.person@spaxe.test", Name: "JIT Person", Status: "active", Tenants: []string{"spaxe"}},
	}}
	auth := galaxyauthsvc.NewBearerAuthenticator(oidc.NewProvider(issuer), env.userRepo, log).
		WithDelegatedJIT(galaxyauthsvc.DelegatedJIT{
			Tenant:      "spaxe",
			Delegators:  []string{"galaxy-gateway"},
			Directory:   dir,
			Provisioner: galaxyauthsvc.New(env.userRepo, env.roleRepo, false, userdom.RoleUser, log),
			Deleted:     env.userRepo,
		})
	token := sign(jwt.MapClaims{"sub": person, "act": map[string]any{"sub": "galaxy-gateway"}, "tenant": "spaxe"})

	t.Run("concurrent first calls converge on one USER account", func(t *testing.T) {
		const n = 12
		var wg sync.WaitGroup
		ids := make([]uuid.UUID, n)
		errs := make([]error, n)
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				u, _, err := auth.AuthenticateBearer(env.ctx, token, http.MethodGet)
				errs[i] = err
				if u != nil {
					ids[i] = u.ID
				}
			}(i)
		}
		close(start)
		wg.Wait()
		for i := range errs {
			if errs[i] != nil {
				t.Fatalf("call %d: %v", i, errs[i])
			}
			if ids[i] != ids[0] {
				t.Fatalf("call %d resolved a different account", i)
			}
		}
		var rows int
		if err := env.db.GetContext(env.ctx, &rows,
			`SELECT count(*) FROM users WHERE oidc_sub = $1 AND deleted_at IS NULL`, person); err != nil {
			t.Fatalf("count: %v", err)
		}
		if rows != 1 {
			t.Fatalf("want exactly one active account, got %d", rows)
		}
		u, err := env.userRepo.FindByOIDCSub(env.ctx, person)
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if u.Role != userdom.RoleUser || u.Email != "jit.person@spaxe.test" {
			t.Fatalf("unexpected account role=%q email=%q", u.Role, u.Email)
		}
	})

	t.Run("never adopts another account holding the same email", func(t *testing.T) {
		const other = "7d0b1c55-7a4e-4a43-9a4f-7d3f0f2b9c11"
		dir.users[other] = &vortexidentity.User{ID: other, Email: "taken@spaxe.test", Status: "active", Tenants: []string{"spaxe"}}
		roleUser, err := env.roleRepo.FindByName(env.ctx, userdom.RoleUser)
		if err != nil {
			t.Fatalf("role: %v", err)
		}
		holder := &userdom.User{
			ID: uuid.New(), Username: "taken-holder", PasswordHash: "x", FullName: "Holder",
			RoleID: roleUser.ID, Email: "taken@spaxe.test", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		if err := env.userRepo.Create(env.ctx, holder); err != nil {
			t.Fatalf("seed holder: %v", err)
		}
		_, _, err = auth.AuthenticateBearer(env.ctx,
			sign(jwt.MapClaims{"sub": other, "act": map[string]any{"sub": "galaxy-gateway"}, "tenant": "spaxe"}),
			http.MethodGet)
		if !errors.Is(err, galaxyauthsvc.ErrDelegatedEmailTaken) {
			t.Fatalf("want ErrDelegatedEmailTaken, got %v", err)
		}
		if _, err := env.userRepo.FindByOIDCSub(env.ctx, other); !errors.Is(err, userdom.ErrNotFound) {
			t.Fatalf("no account may be linked to %s, got %v", other, err)
		}
	})

	t.Run("a deactivated account is not re-created", func(t *testing.T) {
		u, err := env.userRepo.FindByOIDCSub(env.ctx, person)
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if err := env.userRepo.Delete(env.ctx, u.ID); err != nil {
			t.Fatalf("deactivate: %v", err)
		}
		if _, _, err := auth.AuthenticateBearer(env.ctx, token, http.MethodGet); err == nil {
			t.Fatal("a deactivated person must stay rejected")
		}
		var rows int
		if err := env.db.GetContext(env.ctx, &rows, `SELECT count(*) FROM users WHERE oidc_sub = $1`, person); err != nil {
			t.Fatalf("count: %v", err)
		}
		if rows != 1 {
			t.Fatalf("no new row may be written, got %d rows", rows)
		}
	})
}
