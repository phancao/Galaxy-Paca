/**
 * @file oauth.ts — Vortex OIDC login for the SDD Coordination Server UI.
 *
 * Authorization Code + PKCE (S256) against the Galaxy identity service. Enabled
 * only when built with VITE_OIDC=1 (the central server); the per-machine monitor
 * build leaves it off and stays auth-free. Endpoints + redirect are derived from
 * the live origin + the app base path, so the same build works on prod
 * (ai.skyplatform.net) and local without env. Mirrors the hackathon SPA.
 */

const CLIENT_ID = (import.meta.env.VITE_OIDC_CLIENT_ID as string) || "sdd-server";
const TOKEN_KEY = "sdd_access_token";
const VERIFIER_KEY = "sdd_pkce_verifier";
const STATE_KEY = "sdd_oauth_state";
const RETURN_KEY = "sdd_oauth_return";
export function portalUserEmail(): string | null {
  try {
    const t = localStorage.getItem('auth_token');
    if (!t) return null;
    const c = JSON.parse(atob((t.split('.')[1] || '').replace(/-/g, '+').replace(/_/g, '/')));
    return String((c && c.email) || '').toLowerCase() || null;
  } catch { return null; }
}


export const OIDC_ENABLED =
  import.meta.env.VITE_OIDC === "1" || import.meta.env.VITE_OIDC === "true";

function base(): string {
  return import.meta.env.BASE_URL.replace(/\/$/, ""); // e.g. "/sdd-server"
}
function origin(): string {
  return window.location.origin;
}
export function redirectUri(): string {
  return `${origin()}${base()}/callback`;
}
export function callbackPath(): string {
  return `${base()}/callback`;
}

export function getToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_KEY);
  } catch {
    return null;
  }
}
export function clearToken(): void {
  try {
    localStorage.removeItem(TOKEN_KEY);
  } catch {
    /* ignore */
  }
}

/** Decode the stored access token's payload → the logged-in user (no verify). */
export function getUser(): { name: string | null; email: string | null } | null {
  const t = getToken();
  const payloadB64 = t ? t.split(".")[1] : null;
  if (!payloadB64) return null;
  try {
    const p = JSON.parse(
      decodeURIComponent(
        atob(payloadB64.replace(/-/g, "+").replace(/_/g, "/"))
          .split("")
          .map((c) => "%" + c.charCodeAt(0).toString(16).padStart(2, "0"))
          .join("")
      )
    );
    const _pe = portalUserEmail();
    if (_pe && p.email && _pe !== String(p.email).toLowerCase()) { clearToken(); return null; }
    return { name: p.name || null, email: p.email || null };
  } catch {
    return null;
  }
}

/** Clear the local token and end the Vortex session, then return to login. */
export function logout(): void {
  clearToken();
  window.location.href = `${origin()}/login`;
}

function b64url(bytes: Uint8Array): string {
  let s = "";
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}
function rand(byteLen: number): string {
  const a = new Uint8Array(byteLen);
  crypto.getRandomValues(a);
  return b64url(a);
}
async function challenge(verifier: string): Promise<string> {
  const d = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier));
  return b64url(new Uint8Array(d));
}

/** Redirect the browser to Vortex to log in. */
export async function beginLogin(returnPath: string): Promise<void> {
  const verifier = rand(48);
  const state = rand(24);
  const ch = await challenge(verifier);
  try {
    sessionStorage.setItem(VERIFIER_KEY, verifier);
    sessionStorage.setItem(STATE_KEY, state);
    sessionStorage.setItem(RETURN_KEY, returnPath || base() + "/");
  } catch {
    /* ignore */
  }
  const params = new URLSearchParams({
    response_type: "code",
    client_id: CLIENT_ID,
    redirect_uri: redirectUri(),
    scope: "openid email profile roles permissions",
    state,
    code_challenge: ch,
    code_challenge_method: "S256",
  });
  window.location.href = `${origin()}/api/identity/oauth/authorize?${params.toString()}`;
}

/** At the callback: validate state, exchange the code for an access token. */
export async function completeLogin(): Promise<string> {
  const url = new URL(window.location.href);
  const code = url.searchParams.get("code");
  const returnedState = url.searchParams.get("state");
  const verifier = sessionStorage.getItem(VERIFIER_KEY);
  const savedState = sessionStorage.getItem(STATE_KEY);
  const returnPath = sessionStorage.getItem(RETURN_KEY) || base() + "/";
  if (!code) throw new Error("no authorization code");
  if (!savedState || savedState !== returnedState) throw new Error("OAuth state mismatch");
  if (!verifier) throw new Error("missing PKCE verifier");

  const body = new URLSearchParams({
    grant_type: "authorization_code",
    code,
    redirect_uri: redirectUri(),
    client_id: CLIENT_ID,
    code_verifier: verifier,
  });
  const res = await fetch(`${origin()}/api/identity/oauth/token`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: body.toString(),
  });
  if (!res.ok) throw new Error(`token exchange failed: HTTP ${res.status}`);
  const tokens = await res.json();
  try {
    localStorage.setItem(TOKEN_KEY, tokens.access_token);
    sessionStorage.removeItem(VERIFIER_KEY);
    sessionStorage.removeItem(STATE_KEY);
    sessionStorage.removeItem(RETURN_KEY);
  } catch {
    /* ignore */
  }
  window.history.replaceState({}, "", returnPath);
  return returnPath;
}
