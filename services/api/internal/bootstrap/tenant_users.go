package bootstrap

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	userdom "github.com/Paca-AI/api/internal/domain/user"
	"github.com/google/uuid"
)

// The platform's user plane for a tenant it was not routed to.
//
// Why this exists at all, when Paca already has a perfectly good
// `/api/v1/admin/users`: that API authenticates with an API KEY, a key is
// minted by a user through their own session, and a key reaches exactly the
// ONE tenant its `paca_<tenant>_` prefix names. So "keep every tenant's people
// in step with Vortex" needed one long-lived admin credential per tenant,
// sitting in a reconciler's env file — and a tenant could only get its first
// key after a human logged in as an admin that did not exist yet. Seven of
// eight tenants stayed unmanaged for exactly that reason.
//
// These routes remove the requirement instead of working around it. One
// platform secret, no long-lived per-tenant credentials, and the tenant is
// named in the request rather than smuggled in a key prefix.
//
// They are deliberately a NARROW slice of the admin API — list, create,
// update, soft-delete a user, and (in tenant_admin.go) set one global role.
// Everything else an admin can do stays behind a real admin session. The
// reconcilers do their matching, conflict handling and exclusion rules in
// their own code; these routes are the writes, not the policy.
//
// Guards are the same as tenant_admin.go and enforced by the same mux:
// `GALAXY_INTERNAL_SERVICE_SECRET` in constant time, outside `/api/` so the
// gateway never forwards it, tenant refused rather than defaulted, and the
// handler borrows the tenant's own service rather than opening a connection.
//
// SUPER_ADMIN and service accounts: not special-cased here. Setting a role is
// tenant_admin.go's job and it refuses SUPER_ADMIN; `is_service` rows are
// skipped by the reconciler that owns that rule, which is where it can see the
// whole picture.

type tenantUsersHandler struct {
	h *tenantAdminHandler // shares the secret check and the tenant lookup
}

type tenantUserWrite struct {
	Tenant    string `json:"tenant"`
	ID        string `json:"id"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	FullName  string `json:"full_name"`
	Email     string `json:"email"`
	OIDCSub   string `json:"oidc_sub"`
	IsService *bool  `json:"is_service"`
	Role      string `json:"role"`
}

func (u *tenantUsersHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !u.h.enabled() || !u.h.secretOK(r) {
		writeEnvelope(w, http.StatusNotFound, nil, "NOT_FOUND", "not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		u.list(w, r)
	case http.MethodPost:
		u.create(w, r)
	case http.MethodPatch:
		u.update(w, r)
	case http.MethodDelete:
		u.softDelete(w, r)
	default:
		writeEnvelope(w, http.StatusMethodNotAllowed, nil, "METHOD_NOT_ALLOWED",
			"GET, POST, PATCH or DELETE")
	}
}

// tenantOf resolves the tenant named by the request, or writes the refusal.
// Never falls back to the primary: answering for a tenant the caller did not
// name is how a platform write lands in the wrong database.
func (u *tenantUsersHandler) tenantOf(w http.ResponseWriter, code string) (*tenantApp, bool) {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		writeEnvelope(w, http.StatusBadRequest, nil, "INVALID_BODY", "tenant is required")
		return nil, false
	}
	app, ok := u.h.byCode[code]
	if !ok {
		writeEnvelope(w, http.StatusNotFound, nil, "TENANT_NOT_SERVED",
			"this deployment does not serve tenant "+code)
		return nil, false
	}
	if app.userService == nil {
		writeEnvelope(w, http.StatusInternalServerError, nil, "TENANT_NOT_READY",
			"tenant "+code+" has no user service")
		return nil, false
	}
	return app, true
}

func (u *tenantUsersHandler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	app, ok := u.tenantOf(w, q.Get("tenant"))
	if !ok {
		return
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(q.Get("page_size"))
	if size < 1 || size > 200 {
		size = 100
	}
	users, total, err := app.userService.List(r.Context(), page, size)
	if err != nil {
		writeEnvelope(w, http.StatusInternalServerError, nil, "LIST_FAILED", err.Error())
		return
	}
	items := make([]map[string]any, 0, len(users))
	for _, x := range users {
		items = append(items, userView(x))
	}
	writeEnvelope(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": page, "page_size": size,
	}, "", "")
}

func (u *tenantUsersHandler) create(w http.ResponseWriter, r *http.Request) {
	var in tenantUserWrite
	if !decodeBody(w, r, &in) {
		return
	}
	app, ok := u.tenantOf(w, in.Tenant)
	if !ok {
		return
	}
	if strings.TrimSpace(in.Username) == "" || strings.TrimSpace(in.Password) == "" {
		writeEnvelope(w, http.StatusBadRequest, nil, "INVALID_BODY",
			"username and password are required")
		return
	}
	role := strings.ToUpper(strings.TrimSpace(in.Role))
	if role == "" {
		role = userdom.RoleUser
	}
	if role != userdom.RoleUser {
		// A row is created as a plain member and promoted afterwards through
		// tenant_admin.go, which is the one place that decides who is an admin
		// and refuses SUPER_ADMIN. Two doors granting admin is one too many.
		writeEnvelope(w, http.StatusBadRequest, nil, "INVALID_ROLE",
			"new users are created as USER; promote via /internal/tenant-admin")
		return
	}
	created, err := app.userService.Create(r.Context(), userdom.CreateInput{
		Username:  strings.TrimSpace(in.Username),
		Password:  in.Password,
		FullName:  in.FullName,
		Role:      role,
		Email:     strings.TrimSpace(in.Email),
		OIDCSub:   strings.TrimSpace(in.OIDCSub),
		IsService: in.IsService != nil && *in.IsService,
	})
	if err != nil {
		writeEnvelope(w, http.StatusConflict, nil, "CREATE_FAILED", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, userView(created), "", "")
}

func (u *tenantUsersHandler) update(w http.ResponseWriter, r *http.Request) {
	var in tenantUserWrite
	if !decodeBody(w, r, &in) {
		return
	}
	app, ok := u.tenantOf(w, in.Tenant)
	if !ok {
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(in.ID))
	if err != nil {
		writeEnvelope(w, http.StatusBadRequest, nil, "INVALID_BODY", "id must be a uuid")
		return
	}
	if strings.TrimSpace(in.Role) != "" {
		writeEnvelope(w, http.StatusBadRequest, nil, "INVALID_ROLE",
			"role is set via /internal/tenant-admin, not here")
		return
	}
	updated, err := app.userService.AdminUpdate(r.Context(), id, userdom.AdminUpdateInput{
		FullName:  in.FullName,
		Email:     strings.TrimSpace(in.Email),
		OIDCSub:   strings.TrimSpace(in.OIDCSub),
		IsService: in.IsService,
	})
	if err != nil {
		writeEnvelope(w, http.StatusConflict, nil, "UPDATE_FAILED", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, userView(updated), "", "")
}

func (u *tenantUsersHandler) softDelete(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	app, ok := u.tenantOf(w, q.Get("tenant"))
	if !ok {
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(q.Get("id")))
	if err != nil {
		writeEnvelope(w, http.StatusBadRequest, nil, "INVALID_BODY", "id must be a uuid")
		return
	}
	if err := app.userService.Delete(r.Context(), id); err != nil {
		writeEnvelope(w, http.StatusConflict, nil, "DELETE_FAILED", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, map[string]any{"id": id.String(), "deleted": true}, "", "")
}

// userView is the shape both reconcilers read. PasswordHash never leaves the
// domain, and nothing here is not already on the admin API's own user payload.
func userView(x *userdom.User) map[string]any {
	return map[string]any{
		"id":         x.ID.String(),
		"username":   x.Username,
		"full_name":  x.FullName,
		"role":       x.Role,
		"email":      x.Email,
		"oidc_sub":   x.OIDCSub,
		"is_service": x.IsService,
		"deleted_at": x.DeletedAt,
	}
}

func decodeBody(w http.ResponseWriter, r *http.Request, out any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(out); err != nil {
		writeEnvelope(w, http.StatusBadRequest, nil, "INVALID_BODY", "body is not valid JSON")
		return false
	}
	return true
}
