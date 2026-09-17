package bootstrap

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	userdom "github.com/Paca-AI/api/internal/domain/user"
	"github.com/google/uuid"
)

// Platform bootstrap: who is the FIRST admin of a tenant?
//
// Every other admin grant flows Vortex -> app: a person is given the
// `paca_admin` role in Vortex Admin and Galaxy-Authz's reconciler writes ADMIN
// into that tenant's database through the admin API, with an admin API key
// minted for that tenant. That works for every tenant which already has an
// admin. It cannot produce the first one:
//
//   - the admin API needs a key, a key is minted by a user, and only an
//     ADMIN/SUPER_ADMIN can mint one the reconciler may use;
//   - every tenant database seeds its own break-glass `admin`, but a password
//     login carries no credential naming a tenant, so tenantMux sends it to
//     the PRIMARY tenant — the other tenants' seeded admins are unreachable;
//   - OIDC login does land in the right tenant, but creates people as USER.
//
// So a freshly added tenant is a database full of people with no way to
// appoint any of them, and every `paca_admin` grant for it is a no-op that
// reads as "they have not signed in yet". This route is the way out, and it is
// deliberately the smallest one that works: set ONE named person's global role
// in ONE named tenant, and nothing else.
//
// Reaching every tenant from one handler does not undo T7. The handler holds
// no connection of its own — it borrows the tenant's own repositories, the
// same ones that tenant's HTTP handlers use, so which database is written is
// still decided by which tenant graph the code looked up, and a tenant this
// process does not serve is simply absent from the map.
//
// Guards:
//   - GALAXY_INTERNAL_SERVICE_SECRET must be configured AND presented, compared
//     in constant time. Unset means the route does not exist, so a deployment
//     that never joined a platform never grows a new door.
//   - The path is NOT under /api/, and the gateway forwards only /api/*, /ws/*,
//     /storage/*, /plugins*/* and /sdd-api/*. It is therefore reachable over
//     galaxy_network (http://paca-api:8080) and from nowhere public — the same
//     posture as identity's own /internal/*.
//   - SUPER_ADMIN is never touched in either direction: it is the tenant's own
//     seeded owner, and no platform caller may take it or hand it out.
//   - The person is named by `oidc_sub`, the Vortex subject — not by username
//     and not by email. One process now holds eight tenants whose people share
//     localparts, and a name that collides is exactly how a grant meant for one
//     tenant lands on someone in another.
type tenantAdminHandler struct {
	byCode map[string]*tenantApp
	secret string
	log    *slog.Logger
}

type tenantAdminRequest struct {
	Tenant  string `json:"tenant"`
	OIDCSub string `json:"oidc_sub"`
	Role    string `json:"role"`
}

// bootstrapRoles are the only roles this route may write. ADMIN is the point of
// it; USER is here so the same door can undo a grant — a tenant whose only
// admin must be removed would otherwise need a DBA.
var bootstrapRoles = map[string]bool{"ADMIN": true, "USER": true}

func newTenantAdminHandler(byCode map[string]*tenantApp, secret string, log *slog.Logger) *tenantAdminHandler {
	return &tenantAdminHandler{byCode: byCode, secret: secret, log: log}
}

func (h *tenantAdminHandler) enabled() bool { return h != nil && h.secret != "" }

func (h *tenantAdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		writeEnvelope(w, http.StatusNotFound, nil, "NOT_FOUND", "not found")
		return
	}
	if r.Method != http.MethodPost {
		writeEnvelope(w, http.StatusMethodNotAllowed, nil, "METHOD_NOT_ALLOWED", "POST only")
		return
	}
	given := r.Header.Get("X-Service-Secret")
	if subtle.ConstantTimeCompare([]byte(given), []byte(h.secret)) != 1 {
		// The same answer as a deployment without the route: an unauthenticated
		// caller learns nothing about whether this door exists here.
		writeEnvelope(w, http.StatusNotFound, nil, "NOT_FOUND", "not found")
		return
	}

	var req tenantAdminRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeEnvelope(w, http.StatusBadRequest, nil, "INVALID_BODY", "body is not valid JSON")
		return
	}
	code := strings.ToLower(strings.TrimSpace(req.Tenant))
	sub := strings.TrimSpace(req.OIDCSub)
	role := strings.ToUpper(strings.TrimSpace(req.Role))
	if code == "" || sub == "" {
		writeEnvelope(w, http.StatusBadRequest, nil, "INVALID_BODY", "tenant and oidc_sub are required")
		return
	}
	if !bootstrapRoles[role] {
		writeEnvelope(w, http.StatusBadRequest, nil, "INVALID_ROLE", "role must be ADMIN or USER")
		return
	}
	app, ok := h.byCode[code]
	if !ok {
		writeEnvelope(w, http.StatusNotFound, nil, "TENANT_NOT_SERVED",
			"this deployment does not serve tenant "+code)
		return
	}
	if app.users == nil || app.globalRoles == nil {
		writeEnvelope(w, http.StatusInternalServerError, nil, "TENANT_NOT_READY",
			"tenant "+code+" has no user store")
		return
	}

	ctx := r.Context()
	u, err := app.users.FindByOIDCSub(ctx, sub)
	if err != nil {
		// The common, expected miss: the person has never signed in and the
		// directory sync has not created them yet. Saying so plainly is what
		// lets the caller act; a 500 here would just look broken.
		writeEnvelope(w, http.StatusNotFound, nil, "USER_NOT_LINKED",
			"no user in tenant "+code+" carries that Vortex subject")
		return
	}
	if u.Role == "SUPER_ADMIN" {
		writeEnvelope(w, http.StatusConflict, nil, "SUPER_ADMIN_IMMUTABLE",
			"SUPER_ADMIN is the tenant's own owner and is never changed from outside")
		return
	}
	if u.Role == role {
		writeEnvelope(w, http.StatusOK, tenantAdminResult(code, u, role, false), "", "")
		return
	}
	target, err := app.globalRoles.FindByName(ctx, role)
	if err != nil {
		writeEnvelope(w, http.StatusInternalServerError, nil, "ROLE_NOT_FOUND",
			"global role "+role+" does not exist in tenant "+code)
		return
	}
	if err := app.globalRoles.ReplaceUserRoles(ctx, u.ID, []uuid.UUID{target.ID}); err != nil {
		h.log.Error("tenant-admin bootstrap failed",
			"tenant", code, "sub", sub, "role", role, "err", err)
		writeEnvelope(w, http.StatusInternalServerError, nil, "WRITE_FAILED", "could not set the role")
		return
	}
	h.log.Info("tenant-admin bootstrap", "tenant", code, "user", u.Username, "role", role)
	writeEnvelope(w, http.StatusOK, tenantAdminResult(code, u, role, true), "", "")
}

func tenantAdminResult(code string, u *userdom.User, role string, changed bool) map[string]any {
	return map[string]any{
		"tenant":   code,
		"user_id":  u.ID.String(),
		"username": u.Username,
		"role":     role,
		"changed":  changed,
	}
}

// writeEnvelope emits the same {success, data | error_code + error} envelope
// the /api/v1 handlers use, so a caller needs one response shape, not two.
func writeEnvelope(w http.ResponseWriter, status int, data any, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := map[string]any{"success": code == ""}
	if code == "" {
		body["data"] = data
	} else {
		body["error_code"] = code
		body["error"] = msg
	}
	_ = json.NewEncoder(w).Encode(body)
}
