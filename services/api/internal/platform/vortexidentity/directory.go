// Package vortexidentity reads the Vortex identity service's user directory
// (GET /internal/users, authenticated with X-Service-Secret).  It is the
// authoritative answer to "does this Vortex user exist, is it active, and
// which tenants is it a member of" (ADR-040 §2, ADR-058 Đợt 4).
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

// ErrUnknownUser means identity answered but does not list the requested id.
var ErrUnknownUser = errors.New("vortexidentity: user not listed by identity")

// User is one row of GET /internal/users.
type User struct {
	ID     string `json:"id"`
	Email  string `json:"email"`
	Status string `json:"status"`
	Name   string `json:"name"`
	// Tenants holds the codes of the user's ACTIVE tenant memberships.
	Tenants []string `json:"tenants"`
}

// Active reports whether identity lists the user as active.
func (u *User) Active() bool {
	return strings.EqualFold(strings.TrimSpace(u.Status), "active")
}

// MemberOf reports whether the user has an active membership in tenant code.
func (u *User) MemberOf(code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	for _, t := range u.Tenants {
		if strings.EqualFold(strings.TrimSpace(t), code) {
			return true
		}
	}
	return false
}

// Client calls the identity service's internal user directory.
type Client struct {
	baseURL string
	secret  string
	http    *http.Client
}

// New returns a Client for baseURL authenticated with secret.
func New(baseURL, secret string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		secret:  secret,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// FromEnv builds a Client from the same GALAXY_* identity env Paca already
// reads for its AI proxy and the ADR-040 reactivation read-back (with the
// platform-standard fallbacks).  It returns nil when no service secret is
// configured — callers must treat that as "directory unavailable".
func FromEnv() *Client {
	secret := firstNonEmpty(
		os.Getenv("GALAXY_INTERNAL_SERVICE_SECRET"),
		os.Getenv("INTERNAL_SERVICE_SECRET"),
	)
	if secret == "" {
		return nil
	}
	base := firstNonEmpty(
		os.Getenv("GALAXY_IDENTITY_URL"),
		os.Getenv("VORTEX_IDENTITY_URL"),
		os.Getenv("NEXUS_IDENTITY_URL"),
	)
	if base == "" {
		base = "http://vortex-identity:8086"
	}
	return New(base, secret)
}

// LookupUser returns identity's row for userID.  Any transport error, non-200
// answer, or an id identity does not list is an error — never a guessed user.
func (c *Client) LookupUser(ctx context.Context, userID string) (*User, error) {
	if userID == "" {
		return nil, ErrUnknownUser
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/internal/users", nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("ids", userID)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("X-Service-Secret", c.secret)

	resp, err := c.http.Do(req)
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
