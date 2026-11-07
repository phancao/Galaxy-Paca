// Package vortexidentity reads the Vortex identity service's directory over
// its internal API (GET /internal/users, X-Service-Secret).
//
// Two callers ask it the same question — "what does identity say about this
// user right now?" — and must get the same answer:
//
//   - nexussync, before RESTORING a deprovisioned user (ADR-040 audit #3);
//   - galaxyauth, before creating a local account for a person who reached
//     Paca through a delegated bearer token and has never opened the web app
//     (SPA-6550).
//
// Both are the dangerous direction (granting access), so the client is
// fail-closed: no secret, a transport error, a non-200 answer or an id
// identity does not know is never read as "active".
package vortexidentity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// DefaultBaseURL is the in-cluster address of the identity service, used when
// no GALAXY_/VORTEX_/NEXUS_IDENTITY_URL is configured.
const DefaultBaseURL = "http://vortex-identity:8086"

// ErrNotConfigured means no internal service secret is configured, so identity
// cannot be asked at all.
var ErrNotConfigured = errors.New("vortexidentity: no internal service secret configured")

// ErrUnknownUser means identity answered and does not list the user.
var ErrUnknownUser = errors.New("vortexidentity: identity does not list this user")

// User is one row of identity's /internal/users answer.
type User struct {
	ID     string `json:"id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Status string `json:"status"`
	// Tenants are the codes of the user's ACTIVE tenant memberships
	// (ADR-058 Đợt 4).
	Tenants []string `json:"tenants"`
}

// Active reports whether identity lists the user as active.
func (u *User) Active() bool {
	return u != nil && strings.EqualFold(strings.TrimSpace(u.Status), "active")
}

// MemberOf reports whether the user holds an active membership in tenant
// (case-insensitive). An empty tenant is never a membership.
func (u *User) MemberOf(tenant string) bool {
	tenant = strings.ToLower(strings.TrimSpace(tenant))
	if u == nil || tenant == "" {
		return false
	}
	for _, t := range u.Tenants {
		if strings.ToLower(strings.TrimSpace(t)) == tenant {
			return true
		}
	}
	return false
}

// Client calls identity's internal API.
type Client struct {
	BaseURL string
	Secret  string
	HTTP    *http.Client
}

// FromEnv builds a Client from the same GALAXY_* identity env Paca already
// reads for its AI proxy, with the platform-standard fallbacks.
func FromEnv() *Client {
	base := firstNonEmpty(
		os.Getenv("GALAXY_IDENTITY_URL"),
		os.Getenv("VORTEX_IDENTITY_URL"),
		os.Getenv("NEXUS_IDENTITY_URL"),
	)
	if base == "" {
		base = DefaultBaseURL
	}
	return &Client{
		BaseURL: strings.TrimRight(base, "/"),
		Secret: firstNonEmpty(
			os.Getenv("GALAXY_INTERNAL_SERVICE_SECRET"),
			os.Getenv("INTERNAL_SERVICE_SECRET"),
		),
		HTTP: &http.Client{Timeout: 10 * time.Second},
	}
}

// Configured reports whether the client has a secret to authenticate with.
func (c *Client) Configured() bool {
	return c != nil && c.Secret != ""
}

// LookupUser returns identity's row for userID. It returns ErrNotConfigured
// when no secret is set and ErrUnknownUser when identity does not list the id;
// any other failure is returned as an error. It never returns (nil, nil).
func (c *Client) LookupUser(ctx context.Context, userID string) (*User, error) {
	if userID == "" {
		return nil, ErrUnknownUser
	}
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/internal/users", nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("ids", userID)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("X-Service-Secret", c.Secret)

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vortexidentity: /internal/users: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vortexidentity: /internal/users status %d", resp.StatusCode)
	}

	var rows []User
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("vortexidentity: decode /internal/users: %w", err)
	}
	for i := range rows {
		if rows[i].ID == userID {
			return &rows[i], nil
		}
	}
	return nil, ErrUnknownUser
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
