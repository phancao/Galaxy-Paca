package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	userdom "github.com/Paca-AI/api/internal/domain/user"
	"github.com/google/uuid"
)

// memUserService is the tenant's own use-case, faked. It records what the
// platform plane asked for so a test can assert the request landed in THIS
// tenant and nowhere else.
type memUserService struct {
	tenant  string
	rows    []*userdom.User
	created []userdom.CreateInput
	updated []uuid.UUID
	deleted []uuid.UUID
	failNew error
}

func (m *memUserService) GetByID(context.Context, uuid.UUID) (*userdom.User, error) {
	return nil, errors.New("not used")
}

func (m *memUserService) List(_ context.Context, page, size int) ([]*userdom.User, int64, error) {
	start := (page - 1) * size
	if start >= len(m.rows) {
		return nil, int64(len(m.rows)), nil
	}
	end := start + size
	if end > len(m.rows) {
		end = len(m.rows)
	}
	return m.rows[start:end], int64(len(m.rows)), nil
}

func (m *memUserService) ListGlobalPermissions(context.Context, uuid.UUID) ([]string, error) {
	return nil, nil
}

func (m *memUserService) Create(_ context.Context, in userdom.CreateInput) (*userdom.User, error) {
	if m.failNew != nil {
		return nil, m.failNew
	}
	m.created = append(m.created, in)
	return &userdom.User{ID: uuid.New(), Username: in.Username, Role: in.Role,
		Email: in.Email, OIDCSub: in.OIDCSub, IsService: in.IsService}, nil
}

func (m *memUserService) UpdateProfile(context.Context, uuid.UUID, userdom.UpdateProfileInput) (*userdom.User, error) {
	return nil, errors.New("not used")
}

func (m *memUserService) AdminUpdate(_ context.Context, id uuid.UUID, in userdom.AdminUpdateInput) (*userdom.User, error) {
	m.updated = append(m.updated, id)
	return &userdom.User{ID: id, Email: in.Email, OIDCSub: in.OIDCSub, FullName: in.FullName}, nil
}

func (m *memUserService) ResetPassword(context.Context, uuid.UUID, string) error { return nil }
func (m *memUserService) ChangeMyPassword(context.Context, uuid.UUID, string, string) error {
	return nil
}

func (m *memUserService) Delete(_ context.Context, id uuid.UUID) error {
	m.deleted = append(m.deleted, id)
	return nil
}

func usersMux(t *testing.T, secret string, svcs map[string]*memUserService) *tenantMux {
	t.Helper()
	byCode := map[string]*tenantApp{}
	for code, svc := range svcs {
		byCode[code] = &tenantApp{code: code, userService: svc}
	}
	h := newTenantAdminHandler(byCode, secret, slog.Default())
	m := newTenantMux(&tenantApp{code: "galaxy"}, byCode, slog.Default())
	m.internal = h
	m.internalUsers = &tenantUsersHandler{h: h}
	return m
}

func do(t *testing.T, m *tenantMux, method, url, secret, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, url, nil)
	} else {
		r = httptest.NewRequest(method, url, strings.NewReader(body))
	}
	if secret != "" {
		r.Header.Set("X-Service-Secret", secret)
	}
	w := httptest.NewRecorder()
	m.ServeHTTP(w, r)
	return w
}

// --- the door is one door ---------------------------------------------------

func TestUserPlaneSharesTheSameSecretCheck(t *testing.T) {
	m := usersMux(t, testSecret, map[string]*memUserService{"tmo": {tenant: "tmo"}})
	for _, c := range []struct{ name, secret string }{
		{"khong co bi mat", ""}, {"bi mat sai", "sai"},
	} {
		w := do(t, m, http.MethodGet, "/internal/tenant-users?tenant=tmo", c.secret, "")
		if w.Code != http.StatusNotFound || errCode(t, w) != "NOT_FOUND" {
			t.Fatalf("%s: want 404 NOT_FOUND, got %d %s", c.name, w.Code, w.Body.String())
		}
	}
}

func TestUserPlaneIsInvisibleWithoutTheSecretConfigured(t *testing.T) {
	m := usersMux(t, "", map[string]*memUserService{"tmo": {tenant: "tmo"}})
	w := do(t, m, http.MethodGet, "/internal/tenant-users?tenant=tmo", "anything", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// --- tenant isolation: the whole point --------------------------------------

func TestWriteLandsOnlyInTheNamedTenant(t *testing.T) {
	tmo := &memUserService{tenant: "tmo"}
	hdbank := &memUserService{tenant: "hdbank"}
	m := usersMux(t, testSecret, map[string]*memUserService{"tmo": tmo, "hdbank": hdbank})
	w := do(t, m, http.MethodPost, "/internal/tenant-users", testSecret,
		`{"tenant":"tmo","username":"a.nguyen","password":"x","email":"a@tmo.vn","oidc_sub":"sub-a"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", w.Code, w.Body.String())
	}
	if len(tmo.created) != 1 || len(hdbank.created) != 0 {
		t.Fatalf("write leaked: tmo=%d hdbank=%d", len(tmo.created), len(hdbank.created))
	}
	if tmo.created[0].OIDCSub != "sub-a" || tmo.created[0].Email != "a@tmo.vn" {
		t.Fatalf("identity link not passed through: %+v", tmo.created[0])
	}
}

func TestUnknownTenantIsRefusedNotServedByThePrimary(t *testing.T) {
	// Falling back to the primary is how a platform write lands in the wrong
	// database — the failure this whole plane exists to prevent.
	galaxy := &memUserService{tenant: "galaxy"}
	m := usersMux(t, testSecret, map[string]*memUserService{"galaxy": galaxy})
	w := do(t, m, http.MethodPost, "/internal/tenant-users", testSecret,
		`{"tenant":"nosuch","username":"a","password":"x"}`)
	if w.Code != http.StatusNotFound || errCode(t, w) != "TENANT_NOT_SERVED" {
		t.Fatalf("want 404 TENANT_NOT_SERVED, got %d %s", w.Code, w.Body.String())
	}
	if len(galaxy.created) != 0 {
		t.Fatal("refused tenant still wrote into the primary")
	}
}

func TestMissingTenantIsRefused(t *testing.T) {
	m := usersMux(t, testSecret, map[string]*memUserService{"tmo": {tenant: "tmo"}})
	w := do(t, m, http.MethodGet, "/internal/tenant-users", testSecret, "")
	if w.Code != http.StatusBadRequest || errCode(t, w) != "INVALID_BODY" {
		t.Fatalf("want 400 INVALID_BODY, got %d %s", w.Code, w.Body.String())
	}
}

// --- only one door grants admin ---------------------------------------------

func TestUserPlaneCannotCreateAnAdmin(t *testing.T) {
	tmo := &memUserService{tenant: "tmo"}
	m := usersMux(t, testSecret, map[string]*memUserService{"tmo": tmo})
	for _, role := range []string{"ADMIN", "SUPER_ADMIN"} {
		w := do(t, m, http.MethodPost, "/internal/tenant-users", testSecret,
			`{"tenant":"tmo","username":"a","password":"x","role":"`+role+`"}`)
		if w.Code != http.StatusBadRequest || errCode(t, w) != "INVALID_ROLE" {
			t.Fatalf("role %s must be refused here, got %d %s", role, w.Code, w.Body.String())
		}
	}
	if len(tmo.created) != 0 {
		t.Fatal("a refused role still created a row")
	}
}

func TestUserPlaneCannotChangeARole(t *testing.T) {
	tmo := &memUserService{tenant: "tmo"}
	m := usersMux(t, testSecret, map[string]*memUserService{"tmo": tmo})
	w := do(t, m, http.MethodPatch, "/internal/tenant-users", testSecret,
		`{"tenant":"tmo","id":"`+uuid.NewString()+`","role":"ADMIN"}`)
	if w.Code != http.StatusBadRequest || errCode(t, w) != "INVALID_ROLE" {
		t.Fatalf("want 400 INVALID_ROLE, got %d %s", w.Code, w.Body.String())
	}
	if len(tmo.updated) != 0 {
		t.Fatal("a refused role change still updated the row")
	}
}

// --- the ordinary paths -----------------------------------------------------

func TestListPagesAndReturnsTheIdentityColumns(t *testing.T) {
	rows := []*userdom.User{
		{ID: uuid.New(), Username: "a", Role: "USER", Email: "a@tmo.vn", OIDCSub: "sub-a"},
		{ID: uuid.New(), Username: "svc", Role: "USER", IsService: true},
	}
	tmo := &memUserService{tenant: "tmo", rows: rows}
	m := usersMux(t, testSecret, map[string]*memUserService{"tmo": tmo})
	w := do(t, m, http.MethodGet, "/internal/tenant-users?tenant=tmo&page=1&page_size=100", testSecret, "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Data struct {
			Items []map[string]any `json:"items"`
			Total int              `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Total != 2 || len(body.Data.Items) != 2 {
		t.Fatalf("want 2 rows, got %+v", body.Data)
	}
	// The reconciler matches on oidc_sub; if the listing drops it the match
	// silently falls back to usernames, which is the bug this replaced.
	if body.Data.Items[0]["oidc_sub"] != "sub-a" {
		t.Fatalf("oidc_sub missing from the listing: %+v", body.Data.Items[0])
	}
	if body.Data.Items[1]["is_service"] != true {
		t.Fatalf("is_service missing: %+v", body.Data.Items[1])
	}
}

func TestSoftDeleteTargetsTheNamedTenant(t *testing.T) {
	tmo := &memUserService{tenant: "tmo"}
	hdbank := &memUserService{tenant: "hdbank"}
	m := usersMux(t, testSecret, map[string]*memUserService{"tmo": tmo, "hdbank": hdbank})
	id := uuid.NewString()
	w := do(t, m, http.MethodDelete, "/internal/tenant-users?tenant=tmo&id="+id, testSecret, "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", w.Code, w.Body.String())
	}
	if len(tmo.deleted) != 1 || len(hdbank.deleted) != 0 {
		t.Fatalf("delete leaked: tmo=%d hdbank=%d", len(tmo.deleted), len(hdbank.deleted))
	}
}

func TestBadUuidIsRefused(t *testing.T) {
	m := usersMux(t, testSecret, map[string]*memUserService{"tmo": {tenant: "tmo"}})
	w := do(t, m, http.MethodDelete, "/internal/tenant-users?tenant=tmo&id=not-a-uuid", testSecret, "")
	if w.Code != http.StatusBadRequest || errCode(t, w) != "INVALID_BODY" {
		t.Fatalf("want 400, got %d %s", w.Code, w.Body.String())
	}
}

func TestUnsupportedMethodIsRefused(t *testing.T) {
	m := usersMux(t, testSecret, map[string]*memUserService{"tmo": {tenant: "tmo"}})
	w := do(t, m, http.MethodPut, "/internal/tenant-users?tenant=tmo", testSecret, "")
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", w.Code)
	}
}

func TestCreateRequiresUsernameAndPassword(t *testing.T) {
	m := usersMux(t, testSecret, map[string]*memUserService{"tmo": {tenant: "tmo"}})
	w := do(t, m, http.MethodPost, "/internal/tenant-users", testSecret, `{"tenant":"tmo"}`)
	if w.Code != http.StatusBadRequest || errCode(t, w) != "INVALID_BODY" {
		t.Fatalf("want 400 INVALID_BODY, got %d %s", w.Code, w.Body.String())
	}
}
