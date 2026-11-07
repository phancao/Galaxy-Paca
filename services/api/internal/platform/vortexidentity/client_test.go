package vortexidentity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func newIdentity(t *testing.T, status int, rows any) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/internal/users" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Service-Secret") != "s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(rows)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestLookupUserReadsStatusAndTenants(t *testing.T) {
	srv, _ := newIdentity(t, http.StatusOK, []map[string]any{
		{"id": "u-1", "email": "a@x", "name": "A", "status": "active", "tenants": []string{"Spaxe", "galaxy"}},
	})
	c := &Client{BaseURL: srv.URL, Secret: "s3cret", HTTP: srv.Client()}

	u, err := c.LookupUser(context.Background(), "u-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !u.Active() || !u.MemberOf("spaxe") || !u.MemberOf(" GALAXY ") || u.MemberOf("tmo") || u.MemberOf("") {
		t.Fatalf("unexpected reading of %+v", u)
	}
	if u.Email != "a@x" || u.Name != "A" {
		t.Fatalf("email/name not decoded: %+v", u)
	}
}

func TestLookupUserUnknownID(t *testing.T) {
	srv, _ := newIdentity(t, http.StatusOK, []map[string]any{})
	c := &Client{BaseURL: srv.URL, Secret: "s3cret", HTTP: srv.Client()}

	if _, err := c.LookupUser(context.Background(), "u-404"); !errors.Is(err, ErrUnknownUser) {
		t.Fatalf("expected ErrUnknownUser, got %v", err)
	}
}

func TestLookupUserNon200IsAnError(t *testing.T) {
	srv, _ := newIdentity(t, http.StatusServiceUnavailable, map[string]string{"error": "down"})
	c := &Client{BaseURL: srv.URL, Secret: "s3cret", HTTP: srv.Client()}

	u, err := c.LookupUser(context.Background(), "u-1")
	if err == nil || u != nil {
		t.Fatalf("a non-200 answer must be an error, got %+v / %v", u, err)
	}
}

func TestLookupUserWithoutSecretNeverCalls(t *testing.T) {
	srv, calls := newIdentity(t, http.StatusOK, []map[string]any{{"id": "u-1", "status": "active"}})
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	if _, err := c.LookupUser(context.Background(), "u-1"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("no request may be sent without a secret")
	}
}

func TestInactiveStatusIsNotActive(t *testing.T) {
	for _, s := range []string{"suspended", "disabled", "", "pending"} {
		if (&User{Status: s}).Active() {
			t.Fatalf("status %q must not read as active", s)
		}
	}
	if !(&User{Status: " Active "}).Active() {
		t.Fatal(`" Active " must read as active`)
	}
	var nilUser *User
	if nilUser.Active() || nilUser.MemberOf("spaxe") {
		t.Fatal("a nil user is never active nor a member")
	}
}

func TestFromEnvReadsFallbackChain(t *testing.T) {
	t.Setenv("GALAXY_IDENTITY_URL", "")
	t.Setenv("VORTEX_IDENTITY_URL", "http://identity.local:9000/")
	t.Setenv("NEXUS_IDENTITY_URL", "http://ignored")
	t.Setenv("GALAXY_INTERNAL_SERVICE_SECRET", "")
	t.Setenv("INTERNAL_SERVICE_SECRET", "legacy")

	c := FromEnv()
	if c.BaseURL != "http://identity.local:9000" || c.Secret != "legacy" || !c.Configured() {
		t.Fatalf("unexpected client %+v", c)
	}
}
