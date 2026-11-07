package vortexidentity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testUser = "b324afac-8ab6-4d50-8f5b-e86bc5f068d4"

func newIdentity(t *testing.T, status int, rows []map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/users" || r.Header.Get("X-Service-Secret") != "s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("ids") != testUser {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(rows)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLookupUserReturnsDirectoryRow(t *testing.T) {
	srv := newIdentity(t, http.StatusOK, []map[string]any{{
		"id": testUser, "email": "cao.phan@spaxeai.com", "status": "active",
		"name": "Cao Phan", "tenants": []string{"spaxe"},
	}})
	u, err := New(srv.URL+"/", "s3cret").LookupUser(context.Background(), testUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !u.Active() || !u.MemberOf("SPAXE") || u.MemberOf("vtv") || u.MemberOf("") || u.Email != "cao.phan@spaxeai.com" {
		t.Fatalf("unexpected row: %+v", u)
	}
}

func TestLookupUserFailsClosed(t *testing.T) {
	t.Run("unknown id", func(t *testing.T) {
		srv := newIdentity(t, http.StatusOK, []map[string]any{{"id": "someone-else", "status": "active"}})
		if _, err := New(srv.URL, "s3cret").LookupUser(context.Background(), testUser); !errors.Is(err, ErrUnknownUser) {
			t.Fatalf("want ErrUnknownUser, got %v", err)
		}
	})
	t.Run("non-200", func(t *testing.T) {
		srv := newIdentity(t, http.StatusInternalServerError, nil)
		if _, err := New(srv.URL, "s3cret").LookupUser(context.Background(), testUser); err == nil {
			t.Fatal("want error on non-200")
		}
	})
	t.Run("wrong secret", func(t *testing.T) {
		srv := newIdentity(t, http.StatusOK, nil)
		if _, err := New(srv.URL, "nope").LookupUser(context.Background(), testUser); err == nil {
			t.Fatal("want error on 401")
		}
	})
}

func TestFromEnvNeedsSecret(t *testing.T) {
	t.Setenv("GALAXY_INTERNAL_SERVICE_SECRET", "")
	t.Setenv("INTERNAL_SERVICE_SECRET", "")
	if FromEnv() != nil {
		t.Fatal("no secret must mean no directory client")
	}
	t.Setenv("INTERNAL_SERVICE_SECRET", "x")
	t.Setenv("GALAXY_IDENTITY_URL", "http://identity.test/")
	c := FromEnv()
	if c == nil || c.baseURL != "http://identity.test" || c.secret != "x" {
		t.Fatalf("unexpected client: %+v", c)
	}
}
