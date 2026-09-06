// Package auth holds JWT claims and the auth service contract.
package auth

import "github.com/golang-jwt/jwt/v5"

// Claims is the payload embedded in every JWT issued by the service.
type Claims struct {
	jwt.RegisteredClaims
	Username string `json:"username"`
	Role     string `json:"role"`
	// Kind distinguishes "access" from "refresh" tokens.
	Kind string `json:"kind"`
	// FamilyID links all tokens that originate from the same login session.
	// Used for refresh-token rotation and reuse detection.
	FamilyID string `json:"fid"`
	// RememberMe is true when the user opted into a persistent session.
	// Propagated through refresh-token rotation to preserve the original
	// session lifetime preference.
	RememberMe bool `json:"rme"`
	// MustChangePassword is true when the user is required to change their
	// password before accessing any other endpoint (e.g. after admin creation
	// or admin password reset).
	MustChangePassword bool `json:"mcp,omitempty"`
	// AgentID is optionally set when authenticating via agent API key with
	// X-Agent-ID header to identify which agent is performing the action.
	AgentID *string `json:"agent_id,omitempty"`
	// Tenant names the workspace this session belongs to. One process serves
	// several tenants, each with its own database, so the claim is what tells
	// a request which one to go to.
	//
	// It is read UNVERIFIED to pick the tenant, and then verified twice over:
	// the signature covers it, and that tenant's own middleware refuses a
	// token naming anyone else. Flipping it therefore buys nothing — the
	// forged request lands in a graph whose verification it cannot pass.
	//
	// Empty on tokens minted before this claim existed; those belong to the
	// primary tenant, which is where they were issued.
	Tenant string `json:"tenant,omitempty"`
}
