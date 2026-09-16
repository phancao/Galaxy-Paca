package bootstrap

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testSecret = "platform-internal-secret"

func newTestMux(h *tenantAdminHandler) *tenantMux {
	m := newTenantMux(&tenantApp{code: "galaxy"}, map[string]*tenantApp{}, slog.Default())
	m.internal = h
	return m
}

func post(t *testing.T, h http.Handler, secret, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/internal/tenant-admin", strings.NewReader(body))
	if secret != "" {
		r.Header.Set("X-Service-Secret", secret)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func errCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Success   bool   `json:"success"`
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, w.Body.String())
	}
	if body.Success {
		t.Fatalf("expected a failure envelope, got %s", w.Body.String())
	}
	return body.ErrorCode
}

// --- the door itself --------------------------------------------------------

func TestBootstrapRouteIsInvisibleWithoutTheSecretConfigured(t *testing.T) {
	// A deployment that never joined a platform must not grow a new door just
	// by taking the upgrade.
	h := newTenantAdminHandler(map[string]*tenantApp{}, "", slog.Default())
	w := post(t, newTestMux(h), "anything", `{"tenant":"tmo","oidc_sub":"s","role":"ADMIN"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 when the secret is unset, got %d", w.Code)
	}
}

func TestWrongSecretIsIndistinguishableFromNoRoute(t *testing.T) {
	h := newTenantAdminHandler(map[string]*tenantApp{}, testSecret, slog.Default())
	w := post(t, newTestMux(h), "wrong", `{"tenant":"tmo","oidc_sub":"s","role":"ADMIN"}`)
	if w.Code != http.StatusNotFound || errCode(t, w) != "NOT_FOUND" {
		t.Fatalf("a bad secret must look exactly like a missing route, got %d %s",
			w.Code, w.Body.String())
	}
}

func TestMissingSecretHeaderIsRefused(t *testing.T) {
	h := newTenantAdminHandler(map[string]*tenantApp{}, testSecret, slog.Default())
	w := post(t, newTestMux(h), "", `{"tenant":"tmo","oidc_sub":"s","role":"ADMIN"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestGetIsRefused(t *testing.T) {
	h := newTenantAdminHandler(map[string]*tenantApp{}, testSecret, slog.Default())
	r := httptest.NewRequest(http.MethodGet, "/internal/tenant-admin", nil)
	r.Header.Set("X-Service-Secret", testSecret)
	w := httptest.NewRecorder()
	newTestMux(h).ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", w.Code)
	}
}

// --- validation -------------------------------------------------------------

func TestUnknownTenantIsRefusedNotGuessed(t *testing.T) {
	// Falling back to the primary here would write the grant into the WRONG
	// tenant's database — the exact failure the route exists to prevent.
	h := newTenantAdminHandler(map[string]*tenantApp{}, testSecret, slog.Default())
	w := post(t, newTestMux(h), testSecret, `{"tenant":"nosuch","oidc_sub":"s","role":"ADMIN"}`)
	if w.Code != http.StatusNotFound || errCode(t, w) != "TENANT_NOT_SERVED" {
		t.Fatalf("want 404 TENANT_NOT_SERVED, got %d %s", w.Code, w.Body.String())
	}
}

func TestOnlyAdminAndUserMayBeWritten(t *testing.T) {
	h := newTenantAdminHandler(map[string]*tenantApp{"tmo": {code: "tmo"}}, testSecret, slog.Default())
	for _, role := range []string{"SUPER_ADMIN", "OWNER", "", "admin ops"} {
		body, _ := json.Marshal(tenantAdminRequest{Tenant: "tmo", OIDCSub: "s", Role: role})
		w := post(t, newTestMux(h), testSecret, string(body))
		if w.Code != http.StatusBadRequest || errCode(t, w) != "INVALID_ROLE" {
			t.Fatalf("role %q must be refused, got %d %s", role, w.Code, w.Body.String())
		}
	}
}

func TestSuperAdminCannotBeGrantedThroughThisDoor(t *testing.T) {
	// The seeded owner of a tenant is not something the platform hands out.
	h := newTenantAdminHandler(map[string]*tenantApp{"tmo": {code: "tmo"}}, testSecret, slog.Default())
	w := post(t, newTestMux(h), testSecret, `{"tenant":"tmo","oidc_sub":"s","role":"SUPER_ADMIN"}`)
	if errCode(t, w) != "INVALID_ROLE" {
		t.Fatalf("want INVALID_ROLE, got %s", w.Body.String())
	}
}

func TestTenantAndSubjectAreBothRequired(t *testing.T) {
	h := newTenantAdminHandler(map[string]*tenantApp{"tmo": {code: "tmo"}}, testSecret, slog.Default())
	for _, body := range []string{
		`{"oidc_sub":"s","role":"ADMIN"}`,
		`{"tenant":"tmo","role":"ADMIN"}`,
		`{"tenant":"  ","oidc_sub":"  ","role":"ADMIN"}`,
	} {
		w := post(t, newTestMux(h), testSecret, body)
		if w.Code != http.StatusBadRequest || errCode(t, w) != "INVALID_BODY" {
			t.Fatalf("body %s must be refused, got %d %s", body, w.Code, w.Body.String())
		}
	}
}

func TestGarbageBodyIsRefused(t *testing.T) {
	h := newTenantAdminHandler(map[string]*tenantApp{"tmo": {code: "tmo"}}, testSecret, slog.Default())
	w := post(t, newTestMux(h), testSecret, `not json`)
	if w.Code != http.StatusBadRequest || errCode(t, w) != "INVALID_BODY" {
		t.Fatalf("want 400 INVALID_BODY, got %d %s", w.Code, w.Body.String())
	}
}

func TestTenantWithoutRepositoriesFailsLoudlyNotSilently(t *testing.T) {
	h := newTenantAdminHandler(map[string]*tenantApp{"tmo": {code: "tmo"}}, testSecret, slog.Default())
	w := post(t, newTestMux(h), testSecret, `{"tenant":"tmo","oidc_sub":"s","role":"ADMIN"}`)
	if w.Code != http.StatusInternalServerError || errCode(t, w) != "TENANT_NOT_READY" {
		t.Fatalf("want 500 TENANT_NOT_READY, got %d %s", w.Code, w.Body.String())
	}
}

// --- routing ----------------------------------------------------------------

func TestOtherInternalPathsAreNotFoundNotRoutedToATenant(t *testing.T) {
	// /internal/* must never fall through to a tenant graph: a request with no
	// tenant credential would be served by the primary, which is how a
	// platform call silently acts on the wrong tenant.
	h := newTenantAdminHandler(map[string]*tenantApp{}, testSecret, slog.Default())
	m := newTestMux(h)
	r := httptest.NewRequest(http.MethodPost, "/internal/something-else", nil)
	r.Header.Set("X-Service-Secret", testSecret)
	w := httptest.NewRecorder()
	m.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 for an unknown /internal path, got %d", w.Code)
	}
}

func TestTenantAdminPathIsNotUnderApiSoTheGatewayNeverForwardsIt(t *testing.T) {
	// The gateway forwards /api/*, /ws/*, /storage/*, /plugins*/* and
	// /sdd-api/* only. If this path ever moves under /api/ it becomes publicly
	// reachable, and the only guard left is the secret.
	if strings.HasPrefix("/internal/tenant-admin", "/api/") {
		t.Fatal("the bootstrap route must stay outside /api/")
	}
}
