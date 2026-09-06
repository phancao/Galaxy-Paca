package handler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Paca-AI/api/internal/apierr"
	domainauth "github.com/Paca-AI/api/internal/domain/auth"
	userdom "github.com/Paca-AI/api/internal/domain/user"
	"github.com/Paca-AI/api/internal/platform/oidc"
	"github.com/Paca-AI/api/internal/service/galaxyauth"
	"github.com/Paca-AI/api/internal/transport/http/presenter"
)

const (
	// oidcStateCookieName holds the HMAC-signed state+PKCE payload between
	// the login redirect and the issuer callback.
	oidcStateCookieName = "oidc_state"
	// oidcStateCookiePath scopes the cookie to the OIDC endpoints only.
	oidcStateCookiePath = "/api/v1/auth/oidc"
	// oidcStateTTL bounds how long a login attempt may take.
	oidcStateTTL = 10 * time.Minute
	// portalOrigin is where the Vortex session actually lives — the one place
	// a person can change which tenant they are working for. Hard-coded, like
	// every other app in the fleet does it: a door that disappears because a
	// variable was unset is the hardest kind of breakage to notice.
	portalOrigin = "https://ai.skyplatform.net"
)

// OIDCOptions carries the OIDC client settings the handler needs (a transport
// mirror of config.OIDCConfig, kept separate so this package does not import
// the config package).
type OIDCOptions struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       string
	// Tenant is the one Vortex tenant this deployment serves (ADR-058): the
	// callback refuses an id_token that names another tenant, or none.
	Tenant string
}

// SessionIssuer mints a session token pair for an already-authenticated user.
// Satisfied by the auth service so OIDC logins reuse the exact same token
// issuance path as password logins.
type SessionIssuer interface {
	IssueSession(ctx context.Context, u *userdom.User, rememberMe bool) (*domainauth.TokenPair, error)
}

// OIDCUserResolver maps a verified OIDC identity to a local user account.
type OIDCUserResolver interface {
	ResolveOIDCUser(ctx context.Context, id galaxyauth.Identity) (*userdom.User, error)
}

// OIDCHandler implements the Vortex SSO login endpoints (ADR-038):
// GET /auth/oidc/login and GET /auth/oidc/callback.
type OIDCHandler struct {
	provider    *oidc.Provider
	opts        OIDCOptions
	users       OIDCUserResolver
	sessions    SessionIssuer
	auth        *AuthHandler // reused for session cookie writing
	stateSecret []byte
	log         *slog.Logger
}

// NewOIDCHandler returns an OIDCHandler.  stateSecret signs the short-lived
// state cookie; the JWT secret is reused for this purpose.
func NewOIDCHandler(provider *oidc.Provider, opts OIDCOptions, users OIDCUserResolver, sessions SessionIssuer, auth *AuthHandler, stateSecret []byte, log *slog.Logger) *OIDCHandler {
	return &OIDCHandler{
		provider:    provider,
		opts:        opts,
		users:       users,
		sessions:    sessions,
		auth:        auth,
		stateSecret: stateSecret,
		log:         log,
	}
}

// Login handles GET /auth/oidc/login: it stores state + PKCE verifier in a
// signed HttpOnly cookie and redirects the browser to the issuer's
// authorization endpoint.
func (h *OIDCHandler) Login(w http.ResponseWriter, r *http.Request) {
	disc, err := h.provider.Discover(r.Context())
	if err != nil {
		h.log.Error("oidc: discovery failed", "error", err)
		presenter.Error(w, r, apierr.New(apierr.CodeInternalError, "identity provider unavailable"))
		return
	}

	state, err := oidc.RandomToken(32)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	verifier, err := oidc.RandomToken(32)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}

	cookieVal, err := oidc.EncodeLoginState(h.stateSecret, oidc.LoginState{
		State:     state,
		Verifier:  verifier,
		ExpiresAt: time.Now().Add(oidcStateTTL).Unix(),
		Return:    safeReturnPath(r.URL.Query().Get("redirect")),
	})
	if err != nil {
		presenter.Error(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    cookieVal,
		Path:     oidcStateCookiePath,
		HttpOnly: true,
		Secure:   h.auth.cookie.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(oidcStateTTL.Seconds()),
	})

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", h.opts.ClientID)
	q.Set("redirect_uri", h.opts.RedirectURL)
	q.Set("scope", h.opts.Scopes)
	q.Set("state", state)
	q.Set("code_challenge", oidc.PKCEChallengeS256(verifier))
	q.Set("code_challenge_method", "S256")

	sep := "?"
	if strings.Contains(disc.AuthorizationEndpoint, "?") {
		sep = "&"
	}
	http.Redirect(w, r, disc.AuthorizationEndpoint+sep+q.Encode(), http.StatusFound)
}

// Callback handles GET /auth/oidc/callback: it validates state, exchanges the
// code (client_secret_post + PKCE), verifies the RS256 id_token against the
// issuer JWKS, resolves the local user, and issues the same session cookies
// as password login before redirecting to the SPA.
func (h *OIDCHandler) Callback(w http.ResponseWriter, r *http.Request) {
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		h.log.Warn("oidc: issuer returned error", "error", errParam, "description", r.URL.Query().Get("error_description"))
		presenter.Error(w, r, apierr.New(apierr.CodeUnauthenticated, "identity provider rejected the login"))
		return
	}

	code := r.URL.Query().Get("code")
	stateParam := r.URL.Query().Get("state")
	if code == "" || stateParam == "" {
		presenter.Error(w, r, apierr.New(apierr.CodeBadRequest, "missing code or state"))
		return
	}

	stateCookie, err := r.Cookie(oidcStateCookieName)
	if err != nil || stateCookie.Value == "" {
		presenter.Error(w, r, apierr.New(apierr.CodeBadRequest, "missing login state cookie — restart the login flow"))
		return
	}
	h.clearStateCookie(w)

	loginState, err := oidc.DecodeLoginState(h.stateSecret, stateCookie.Value)
	if err != nil {
		h.log.Warn("oidc: state cookie rejected", "error", err)
		presenter.Error(w, r, apierr.New(apierr.CodeBadRequest, "invalid login state — restart the login flow"))
		return
	}
	if subtle.ConstantTimeCompare([]byte(loginState.State), []byte(stateParam)) != 1 {
		presenter.Error(w, r, apierr.New(apierr.CodeBadRequest, "state mismatch — restart the login flow"))
		return
	}

	idToken, err := h.exchangeCode(r.Context(), code, loginState.Verifier)
	if err != nil {
		h.log.Error("oidc: code exchange failed", "error", err)
		presenter.Error(w, r, apierr.New(apierr.CodeUnauthenticated, "token exchange with identity provider failed"))
		return
	}

	claims, err := h.provider.VerifyToken(r.Context(), idToken, h.opts.ClientID)
	if err != nil {
		h.log.Warn("oidc: id_token rejected", "error", err)
		presenter.Error(w, r, apierr.New(apierr.CodeTokenInvalid, "invalid id_token"))
		return
	}

	// ADR-058 Đợt 4: the token names the tenant the person CHOSE at the
	// portal. This deployment serves exactly one tenant, so anything else —
	// or a token naming none — is refused here, before a local user exists.
	tenant := effectiveTenant(claims)
	if tenant == "" {
		presenter.Error(w, r, apierr.New(apierr.CodeUnauthenticated, "TENANT_REQUIRED: the Vortex session names no tenant"))
		return
	}
	if tenant != h.opts.Tenant {
		h.log.Warn("oidc: tenant mismatch", "token_tenant", tenant, "deployment_tenant", h.opts.Tenant)
		// Người dùng đang ở TRÌNH DUYỆT, giữa một lần đăng nhập. Trả JSON
		// {"code":"FORBIDDEN"} ra màn hình là đúng sự thật mà vô dụng: nó
		// không nói vì sao, và không có lối ra. Câu trả lời đúng là một
		// trang nói rõ chuyện gì đã xảy ra kèm đường quay lại.
		h.renderTenantMismatch(w, tenant)
		return
	}

	identity := galaxyauth.Identity{
		Subject:           stringClaim(claims, "sub"),
		Email:             stringClaim(claims, "email"),
		Name:              stringClaim(claims, "name"),
		PreferredUsername: stringClaim(claims, "preferred_username"),
	}

	user, err := h.users.ResolveOIDCUser(r.Context(), identity)
	if err != nil {
		if errors.Is(err, galaxyauth.ErrUserNotProvisioned) {
			presenter.Error(w, r, apierr.New(apierr.CodeUnauthenticated, "no local account for this identity and auto-provisioning is disabled"))
			return
		}
		h.log.Error("oidc: user resolution failed", "error", err)
		presenter.Error(w, r, apierr.New(apierr.CodeInternalError, "could not resolve user account"))
		return
	}

	pair, err := h.sessions.IssueSession(r.Context(), user, true)
	if err != nil {
		presenter.Error(w, r, err)
		return
	}

	h.auth.setTokenCookies(w, pair, pair.RefreshTTL)
	h.log.Info("oidc: SSO login", "user_id", user.ID, "username", user.Username)
	// Back to where they were headed before the login interrupted them. This
	// used to be a hard-coded "/": following a deep link meant signing in and
	// arriving at the home page, which reads as a login that did not work.
	// The path came out of the HMAC-signed state, and is re-checked anyway.
	http.Redirect(w, r, orRoot(safeReturnPath(loginState.Return)), http.StatusFound)
}

// exchangeCode redeems the authorization code at the issuer's token endpoint
// using client_secret_post plus the PKCE verifier, returning the raw id_token.
func (h *OIDCHandler) exchangeCode(ctx context.Context, code, verifier string) (string, error) {
	disc, err := h.provider.Discover(ctx)
	if err != nil {
		return "", err
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", h.opts.RedirectURL)
	form.Set("client_id", h.opts.ClientID)
	form.Set("client_secret", h.opts.ClientSecret)
	form.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, disc.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		// Deliberately omit the body: error payloads from misconfigured
		// issuers can echo credentials.
		return "", fmt.Errorf("token endpoint returned status %d", resp.StatusCode)
	}

	var tokenResp struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tokenResp.IDToken == "" {
		return "", fmt.Errorf("token response contained no id_token")
	}
	return tokenResp.IDToken, nil
}

// safeReturnPath keeps only a path on THIS site, and returns "" for anything
// else. Two rules carry the weight: it must start with a single "/" (so
// "https://evil" and the protocol-relative "//evil" are both rejected, which
// is what turns a login into an open redirect), and it must not smuggle CR/LF
// into the Location header.
func safeReturnPath(v string) string {
	if v == "" || !strings.HasPrefix(v, "/") || strings.HasPrefix(v, "//") {
		return ""
	}
	if strings.ContainsAny(v, "\r\n") {
		return ""
	}
	return v
}

// renderTenantMismatch answers the browser with a readable page instead of an
// error envelope. Status stays 403 — the request really was refused — but the
// body is for the person, not for a client library.
//
// This deployment serves exactly one tenant (ADR-058). Someone who switched
// workspace at the portal and then opened this app is not doing anything
// wrong; they are simply somewhere that does not exist for them yet. So the
// page names both tenants and offers the only two moves that help: change
// workspace back, or go to the portal.
func (h *OIDCHandler) renderTenantMismatch(w http.ResponseWriter, sessionTenant string) {
	switchURL := portalOrigin + "/nexus/switch-workspace?return_url=" + url.QueryEscape(h.publicOrigin())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	fmt.Fprintf(w, `<!doctype html><html lang="vi"><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Sai nơi làm việc</title>
<style>
:root{color-scheme:light dark}
body{margin:0;min-height:100vh;display:grid;place-items:center;
 font:16px/1.6 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;
 background:#0b0e14;color:#e6e8ee}
.card{max-width:34rem;padding:2.5rem;text-align:center}
h1{font-size:1.5rem;margin:0 0 1rem}
p{margin:0 0 1.25rem;color:#a9b0c0}
b{color:#e6e8ee;font-weight:600}
a.btn{display:inline-block;padding:.7rem 1.4rem;border-radius:.6rem;
 background:#4f7cff;color:#fff;text-decoration:none;font-weight:600}
a.sub{display:inline-block;margin-top:1rem;color:#8f97a8;font-size:.9rem}
</style>
<div class="card">
<h1>Không gian này phục vụ một nơi làm việc khác</h1>
<p>Ứng dụng đang chạy cho <b>%s</b>, còn phiên của bạn đang ở <b>%s</b>.
Đổi nơi làm việc rồi quay lại là xong.</p>
<a class="btn" href="%s">Đổi nơi làm việc</a>
<div><a class="sub" href="%s">Về Vortex</a></div>
</div>`,
		html.EscapeString(h.opts.Tenant),
		html.EscapeString(sessionTenant),
		html.EscapeString(switchURL),
		portalOrigin,
	)
}

// publicOrigin is where a browser reaches THIS deployment — derived from the
// configured redirect URL, which is the one absolute URL of ours that identity
// already had to be told about.
func (h *OIDCHandler) publicOrigin() string {
	u, err := url.Parse(h.opts.RedirectURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return portalOrigin
	}
	return u.Scheme + "://" + u.Host
}

func orRoot(v string) string {
	if v == "" {
		return "/"
	}
	return v
}

func (h *OIDCHandler) clearStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    "",
		Path:     oidcStateCookiePath,
		HttpOnly: true,
		Secure:   h.auth.cookie.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func stringClaim(claims map[string]any, key string) string {
	v, _ := claims[key].(string)
	return v
}

// effectiveTenant mirrors galaxy_auth.effective_tenant: `act_as_tenant`, else
// `tenant`, trimmed and lower-cased. Whitespace counts as absent, and the home
// tenant (`primary_org_id`) is never read — that fallback is what ADR-058
// removes.
func effectiveTenant(claims map[string]any) string {
	for _, key := range []string{"act_as_tenant", "tenant"} {
		if v := strings.ToLower(strings.TrimSpace(stringClaim(claims, key))); v != "" {
			return v
		}
	}
	return ""
}
