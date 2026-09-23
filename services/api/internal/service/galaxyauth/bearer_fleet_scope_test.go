package galaxyauth

import (
	"context"
	"net/http"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// PACA-C1's foreign-resource rule was written before the aggregating MCP
// gateway existed (Vortex ADR-043: one endpoint, one login, no per-server OAuth
// storm). The gateway forwards the caller's ORIGINAL token unchanged, and that
// token's scopes are `mcp:galaxy:read`/`:write` — same `mcp:` family as Paca's
// own, none with the `mcp:paca:` prefix. So the rule fired on exactly the
// intended path: every call 401'd with "scope does not grant access to this
// resource".
//
// Measured on SpaxeAI 24/09/2026, and it hid behind a different failure: the
// token's `iss` did not match GALAXY_TRUSTED_ISSUER either, so the first error
// said "unknown principal" and this one only surfaced after that was fixed.

func TestFleetScopeIsNotAForeignResource(t *testing.T) {
	auth, store, sign := newBearerFixture(t)
	auth.WithResourceScopePrefix("mcp:paca:").WithFleetScopePrefix("mcp:galaxy:")
	seedUser(store, "cao-sub")

	token := sign(jwt.MapClaims{"sub": "cao-sub", "scope": "mcp:galaxy:read mcp:galaxy:write"})
	if _, _, err := auth.AuthenticateBearer(context.Background(), token, http.MethodGet); err != nil {
		t.Fatalf("fleet-scoped token must reach Paca, got %v", err)
	}
}

func TestFleetScopeStillSeparatesReadFromWrite(t *testing.T) {
	auth, store, sign := newBearerFixture(t)
	auth.WithResourceScopePrefix("mcp:paca:").WithFleetScopePrefix("mcp:galaxy:")
	seedUser(store, "cao-sub")

	token := sign(jwt.MapClaims{"sub": "cao-sub", "scope": "mcp:galaxy:read"})
	if _, _, err := auth.AuthenticateBearer(context.Background(), token, http.MethodGet); err != nil {
		t.Fatalf("read scope should be accepted on GET, got %v", err)
	}
	for _, m := range []string{http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete} {
		if _, _, err := auth.AuthenticateBearer(context.Background(), token, m); err == nil {
			t.Fatalf("fleet READ scope must be denied on %s — widening the gate must not widen it to writes", m)
		}
	}
}

// The whole point of the foreign-resource rule survives: a token minted for
// another server is still refused. Widening the gate to the fleet must not
// widen it to everything that happens to start with "mcp:".
func TestAnotherServersScopeIsStillForeign(t *testing.T) {
	auth, store, sign := newBearerFixture(t)
	auth.WithResourceScopePrefix("mcp:paca:").WithFleetScopePrefix("mcp:galaxy:")
	seedUser(store, "cao-sub")

	token := sign(jwt.MapClaims{"sub": "cao-sub", "scope": "mcp:wiki:read mcp:wiki:write"})
	if _, _, err := auth.AuthenticateBearer(context.Background(), token, http.MethodGet); err == nil {
		t.Fatal("a token scoped to another MCP server must still be refused")
	}
}

// Unconfigured fleet prefix keeps the old behaviour exactly — a deployment
// that never sets it must not silently start accepting fleet tokens.
func TestUnsetFleetPrefixKeepsTheOldRule(t *testing.T) {
	auth, store, sign := newBearerFixture(t)
	auth.WithResourceScopePrefix("mcp:paca:")
	seedUser(store, "cao-sub")

	token := sign(jwt.MapClaims{"sub": "cao-sub", "scope": "mcp:galaxy:read"})
	if _, _, err := auth.AuthenticateBearer(context.Background(), token, http.MethodGet); err == nil {
		t.Fatal("without WithFleetScopePrefix the fleet scope stays foreign")
	}
}

// A token carrying BOTH is accepted, and the Paca-specific write scope lifts
// the read-only denial the fleet read scope would impose alone.
func TestPacaWriteScopeBesideFleetReadStillWrites(t *testing.T) {
	auth, store, sign := newBearerFixture(t)
	auth.WithResourceScopePrefix("mcp:paca:").WithFleetScopePrefix("mcp:galaxy:")
	seedUser(store, "cao-sub")

	token := sign(jwt.MapClaims{"sub": "cao-sub", "scope": "mcp:galaxy:read mcp:paca:write"})
	if _, _, err := auth.AuthenticateBearer(context.Background(), token, http.MethodPost); err != nil {
		t.Fatalf("an explicit Paca write scope must still grant writes, got %v", err)
	}
}

// Paca duoc doi ten tu `pm`. Cuoc doi ten di toi MOI TEN HAM client nhin thay
// (`pm__*` bien mat, co y), nhung KHONG di toi so scope cua identity: no van
// phat `mcp:pm:read`/`:write` va khong he co dong `mcp:paca:*`. Nen PACA-C1
// doc chinh TEN CU cua minh nhu mot tai nguyen la va tu choi moi luot goi.
//
// Do 24/09/2026 tren SpaxeAI, doc duoc nguyen van sau khi cau tu choi chiu
// neu dich danh scope no thay:
//
//	token carries [mcp:design:read … mcp:pm:read mcp:pm:write … mcp:wiki:write];
//	need prefix "mcp:paca:" or fleet prefix "mcp:galaxy:"

func TestFormerNameIsStillThisService(t *testing.T) {
	auth, store, sign := newBearerFixture(t)
	auth.WithResourceScopePrefix("mcp:paca:").
		WithFleetScopePrefix("mcp:galaxy:").
		WithLegacyScopePrefixes([]string{"mcp:pm:"})
	seedUser(store, "cao-sub")

	token := sign(jwt.MapClaims{"sub": "cao-sub", "scope": "mcp:wiki:read mcp:pm:read mcp:pm:write"})
	if _, _, err := auth.AuthenticateBearer(context.Background(), token, http.MethodGet); err != nil {
		t.Fatalf("the service's own former scope name must reach it, got %v", err)
	}
}

func TestFormerNameStillSeparatesReadFromWrite(t *testing.T) {
	auth, store, sign := newBearerFixture(t)
	auth.WithResourceScopePrefix("mcp:paca:").WithLegacyScopePrefixes([]string{"mcp:pm:"})
	seedUser(store, "cao-sub")

	token := sign(jwt.MapClaims{"sub": "cao-sub", "scope": "mcp:pm:read"})
	if _, _, err := auth.AuthenticateBearer(context.Background(), token, http.MethodGet); err != nil {
		t.Fatalf("legacy read scope should be accepted on GET, got %v", err)
	}
	if _, _, err := auth.AuthenticateBearer(context.Background(), token, http.MethodPost); err == nil {
		t.Fatal("a legacy READ scope must still be denied on POST — honouring an old name is not widening it")
	}
}

func TestUnsetLegacyPrefixesKeepsTheOldRule(t *testing.T) {
	auth, store, sign := newBearerFixture(t)
	auth.WithResourceScopePrefix("mcp:paca:").WithLegacyScopePrefixes(nil)
	seedUser(store, "cao-sub")

	token := sign(jwt.MapClaims{"sub": "cao-sub", "scope": "mcp:pm:read"})
	if _, _, err := auth.AuthenticateBearer(context.Background(), token, http.MethodGet); err == nil {
		t.Fatal("without WithLegacyScopePrefixes the former name stays foreign")
	}
}

func TestEmptyLegacyEntriesAreDroppedNotMatchEverything(t *testing.T) {
	// A blank entry would be a prefix of EVERY string — the whole gate would
	// open on a stray comma in the env var.
	auth, store, sign := newBearerFixture(t)
	auth.WithResourceScopePrefix("mcp:paca:").
		WithLegacyScopePrefixes([]string{"", "  ", "mcp:pm:"})
	seedUser(store, "cao-sub")

	token := sign(jwt.MapClaims{"sub": "cao-sub", "scope": "mcp:wiki:read"})
	if _, _, err := auth.AuthenticateBearer(context.Background(), token, http.MethodGet); err == nil {
		t.Fatal("a blank legacy prefix must not match every scope")
	}
}
