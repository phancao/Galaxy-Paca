package bootstrap

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

// tenantMux sends each request to the dependency graph of the tenant it
// belongs to.
//
// It reads the tenant from the request WITHOUT verifying anything, and that
// is safe for one reason worth being explicit about: routing is not a
// decision about access. Whichever graph a request lands in then authenticates
// it from scratch, and that graph refuses any token not minted for its own
// tenant (jwttoken.Manager.Verify). Pointing a request at the wrong tenant
// therefore buys nothing but a 401 — the same 401 it would get by asking
// directly.
//
// Reading the claim unverified here, instead of verifying twice, keeps the
// signing key out of the multiplexer and leaves exactly one place in the
// process that decides whether a caller is who they say: the tenant's own
// middleware.
type tenantMux struct {
	primary *tenantApp
	byCode  map[string]*tenantApp
	log     *slog.Logger
}

func newTenantMux(primary *tenantApp, byCode map[string]*tenantApp, log *slog.Logger) *tenantMux {
	return &tenantMux{primary: primary, byCode: byCode, log: log}
}

func (m *tenantMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.pick(r).handler.ServeHTTP(w, r)
}

// pick resolves the tenant graph for a request, falling back to the primary.
//
// The fallback is not a guess about identity — an unattributable request is
// either public (health, the login redirect) or about to be refused. What it
// must never be is a request served by a tenant it did not name, and it
// cannot be: a named tenant that this deployment serves always wins here, and
// a token naming a tenant we do NOT serve is refused by the primary's own
// verification rather than quietly answered.
func (m *tenantMux) pick(r *http.Request) *tenantApp {
	code := tenantFromRequest(r)
	if code == "" {
		return m.primary
	}
	if app, ok := m.byCode[code]; ok {
		return app
	}
	// A tenant we do not serve. The OIDC callback is the one place that can
	// say something useful about it (a page explaining which workspace this
	// deployment serves), and that handler lives on the primary.
	return m.primary
}

// tenantFromRequest reads the tenant a request claims to belong to, from
// whichever credential it carries. Returns "" when the request names none.
func tenantFromRequest(r *http.Request) string {
	// Paca's own session: the cookie the browser sends on every call.
	if c, err := r.Cookie("access_token"); err == nil && c.Value != "" {
		if t := tenantFromJWT(c.Value); t != "" {
			return t
		}
	}

	header := r.Header.Get("Authorization")
	if header != "" {
		parts := strings.SplitN(header, " ", 2)
		if len(parts) == 2 {
			switch strings.ToLower(parts[0]) {
			case "bearer":
				// Two token shapes share this header. Paca's own HS256
				// session carries `tenant`; a Vortex RS256 token (ADR-038)
				// carries the tenant the person CHOSE at the portal, under
				// act_as_tenant or tenant. Both are read the same way.
				if t := tenantFromJWT(parts[1]); t != "" {
					return t
				}
			case "apikey":
				if t := tenantFromAPIKey(parts[1]); t != "" {
					return t
				}
			}
		}
	}
	if v := r.Header.Get("X-API-Key"); v != "" {
		if t := tenantFromAPIKey(v); t != "" {
			return t
		}
	}
	return ""
}

// tenantFromJWT decodes a JWT payload without verifying it and returns the
// tenant it names: act_as_tenant first (a delegated call acts THERE), then
// tenant. Any parse failure returns "" — an unreadable token is not a routing
// decision, it is a 401 waiting to happen in whichever graph receives it.
func tenantFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		ActAsTenant string `json:"act_as_tenant"`
		Tenant      string `json:"tenant"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	if claims.ActAsTenant != "" {
		return strings.ToLower(strings.TrimSpace(claims.ActAsTenant))
	}
	return strings.ToLower(strings.TrimSpace(claims.Tenant))
}

// tenantFromAPIKey reads the tenant a key was minted for.
//
// New keys are `paca_<tenant>_<random>` — three parts. Keys minted before the
// tenant existed are `paca_<random>` — two parts — and return "", which lands
// them on the primary: where they were created, and the only database whose
// rows they hash to.
//
// Counting the parts is what separates the two shapes, rather than guessing
// from the alphabet: a random hex tail is also lowercase letters and digits,
// so `paca_deadbeef…` would otherwise read as a tenant called "deadbeef".
func tenantFromAPIKey(key string) string {
	parts := strings.Split(key, "_")
	if len(parts) != 3 || parts[0] != "paca" || parts[1] == "" {
		return ""
	}
	code := strings.ToLower(parts[1])
	for _, c := range code {
		if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') && c != '-' {
			return ""
		}
	}
	return code
}
