"use strict";
/**
 * sdd-proxy — same-origin data proxy (ADR-038): `/sdd-api/*` on the Paca
 * origin (tasks.skyplatform.net) -> the SDD Coordination Server READ API
 * (sdd-server:4830, routes under `/api/*`), over galaxy_network.
 *
 * WHY THIS EXISTS
 * ---------------
 * The native `com.galaxy.sdd` plugin renders SDD fleet telemetry INSIDE Paca
 * (no more iframe to ai.skyplatform.net/sdd-server). A browser on the Paca
 * origin holds a PACA session cookie, not a Vortex OIDC access token — but
 * SDD's read API (central/auth.js `requireRead`) needs a Bearer identity. So
 * this proxy bridges the two, and it does exactly three things per request:
 *
 *   1. SESSION GATE. It forwards the caller's own Cookie to Paca's
 *      `GET /api/v1/users/me`; a non-2xx answer means "no valid Paca session"
 *      and the proxy returns 401 without ever touching SDD. The proxy carries
 *      NO user credential of its own — you see SDD data only while logged into
 *      Paca. (Fleet telemetry is TEAM-WIDE, not tenant-secret, so any
 *      logged-in member may read it — ADR-038.)
 *
 *   2. SERVICE TOKEN. It injects a short-lived RS256 SERVICE token minted at
 *      identity's `/internal/mint-service-token` (iss=galaxy-nexus, TTL<=900s,
 *      verified by SDD via JWKS) — mirroring the dock-trigger mint. A leaked
 *      INTERNAL_SERVICE_SECRET can only mint a non-privileged token, so the
 *      blast radius is a team-wide READ.
 *
 *      There is NO fallback. An earlier version self-signed an HS256 token with
 *      the fleet-shared JWT_SECRET when the mint failed, so a brief identity
 *      outage silently swapped an asymmetric credential only identity can issue
 *      for a symmetric one every container in the fleet can forge — and it did
 *      so without saying anything. A failed mint now returns 502
 *      TOKEN_UNAVAILABLE and the SDD panel shows an error.
 *
 *   3. READ-ONLY REVERSE PROXY. `GET /sdd-api/<x>` -> `sdd-server:4830/api/<x>`,
 *      streaming the JSON back unchanged. Only GET/HEAD are allowed: a write
 *      carried by a service token would lose per-user attribution, and the
 *      human task board lives in Paca now (ADR-038 T6).
 *
 * Secrets (INTERNAL_SERVICE_SECRET, the minted tokens themselves) are NEVER
 * logged. This file has ZERO npm dependencies (Node 20 built-ins only).
 */

const http = require("http");

// ── Config (env, with in-cluster defaults) ──────────────────────────────────
const PORT = parseInt(process.env.PORT || "8791", 10);
const SDD_UPSTREAM = (process.env.SDD_UPSTREAM_URL || "http://sdd-server:4830").replace(/\/+$/, "");
const IDENTITY_URL = (process.env.GALAXY_IDENTITY_URL || "http://vortex-identity:8086").replace(/\/+$/, "");
const PACA_AUTH_URL = process.env.PACA_AUTH_CHECK_URL || "http://api:8080/api/v1/users/me";
const SERVICE_SECRET = process.env.GALAXY_INTERNAL_SERVICE_SECRET || "";
const SERVICE_SUB = process.env.SDD_SERVICE_SUB || "svc-paca-sdd-fleet";
const SERVICE_AUD = process.env.SDD_SERVICE_AUD || "sdd-server";
const MINT_TTL = 900; // seconds; identity caps at 900 anyway

function log(msg) {
  // Timestamped, secret-free operational logging.
  process.stdout.write(`[sdd-proxy] ${new Date().toISOString()} ${msg}\n`);
}

// ── JSON response helper ─────────────────────────────────────────────────────
function sendJson(res, status, obj) {
  const body = Buffer.from(JSON.stringify(obj));
  res.writeHead(status, {
    "Content-Type": "application/json; charset=utf-8",
    "Content-Length": body.length,
    "Cache-Control": "no-store",
  });
  res.end(body);
}

// ── Service-token cache (RS256 minted at identity; no fallback) ──────────────
let cached = { token: null, exp: 0 };

async function mintRs256() {
  if (!SERVICE_SECRET) return null;
  let r;
  try {
    r = await fetch(`${IDENTITY_URL}/internal/mint-service-token`, {
      method: "POST",
      headers: { "X-Service-Secret": SERVICE_SECRET, "Content-Type": "application/json" },
      body: JSON.stringify({
        sub: SERVICE_SUB,
        aud: SERVICE_AUD,
        ttl_seconds: MINT_TTL,
        extra: { name: "Paca SDD Fleet Proxy" },
      }),
    });
  } catch (e) {
    log(`RS256 mint unreachable: ${e.message}`);
    return null;
  }
  if (r.status !== 200) {
    log(`RS256 mint failed: HTTP ${r.status}`);
    return null;
  }
  const j = await r.json().catch(() => null);
  const tok = j && j.access_token;
  if (!tok) return null;
  return { token: tok, ttl: (j && j.expires_in) || MINT_TTL };
}

async function getServiceToken() {
  const now = Math.floor(Date.now() / 1000);
  if (cached.token && now < cached.exp - 60) return cached.token;
  // RS256 via identity mint (mirrors dock-trigger). If that fails we return
  // null and the request 502s. We do NOT substitute a weaker credential: a
  // self-signed HS256 token would keep the panel green while the fleet quietly
  // ran on a key any container can forge, which is a worse outcome than an
  // honest error on a read-only telemetry panel.
  const minted = await mintRs256();
  if (!minted) return null;
  cached = { token: minted.token, exp: now + minted.ttl };
  return cached.token;
}

// ── Paca session gate: 200 authed / 401 anon / 502 auth-backend-broken ───────
async function checkSession(cookie) {
  if (!cookie) return 401;
  let r;
  try {
    r = await fetch(PACA_AUTH_URL, { headers: { Cookie: cookie, Accept: "application/json" } });
  } catch (e) {
    log(`session check unreachable: ${e.message}`);
    return 502;
  }
  if (r.status >= 200 && r.status < 300) return 200;
  if (r.status === 401 || r.status === 403) return 401;
  log(`session check unexpected: HTTP ${r.status}`);
  return 502;
}

// ── Map an inbound path to the SDD upstream path (always under /api) ─────────
// Handles both "/sdd-api/team/overview" (Caddy handle, prefix kept) and
// "/team/overview" (Caddy handle_path, prefix stripped). Result: "/api/...".
function toUpstreamPath(url) {
  let rest = url;
  if (rest.startsWith("/sdd-api")) rest = rest.slice("/sdd-api".length);
  if (rest === "" || rest === "/") return "/api/health";
  if (rest.startsWith("/api/") || rest.startsWith("/api?")) return rest;
  if (!rest.startsWith("/")) rest = "/" + rest;
  return "/api" + rest;
}

// ── Server ───────────────────────────────────────────────────────────────────
const server = http.createServer(async (req, res) => {
  try {
    // Container/monitoring healthcheck — no session, no upstream.
    if (req.url === "/healthz" || req.url === "/sdd-api/healthz") {
      return sendJson(res, 200, { status: "ok", service: "sdd-proxy" });
    }

    // READ-ONLY: a write carried by a service token would lose user attribution.
    if (req.method !== "GET" && req.method !== "HEAD") {
      return sendJson(res, 405, {
        error: { code: "READ_ONLY", message: "the sdd-api proxy serves GET reads only" },
      });
    }

    // 1) Session gate — you see SDD data only while logged into Paca.
    const sess = await checkSession(req.headers.cookie);
    if (sess === 401)
      return sendJson(res, 401, {
        error: { code: "UNAUTHENTICATED", message: "a valid Paca session is required" },
      });
    if (sess === 502)
      return sendJson(res, 502, {
        error: { code: "AUTH_UNAVAILABLE", message: "could not verify the Paca session" },
      });

    // 2) Service token (cached, refreshed before expiry).
    const token = await getServiceToken();
    if (!token)
      return sendJson(res, 502, {
        error: { code: "TOKEN_UNAVAILABLE", message: "could not obtain an SDD service token" },
      });

    // 3) Read-only reverse proxy to the SDD read API.
    const upstreamPath = toUpstreamPath(req.url);
    let upstream;
    try {
      upstream = await fetch(`${SDD_UPSTREAM}${upstreamPath}`, {
        method: "GET",
        headers: { Authorization: `Bearer ${token}`, Accept: "application/json" },
      });
    } catch (e) {
      log(`upstream unreachable ${upstreamPath}: ${e.message}`);
      return sendJson(res, 502, {
        error: { code: "UPSTREAM_UNREACHABLE", message: "sdd-server did not respond" },
      });
    }

    const buf = Buffer.from(await upstream.arrayBuffer());
    res.writeHead(upstream.status, {
      "Content-Type": upstream.headers.get("content-type") || "application/json; charset=utf-8",
      "Content-Length": buf.length,
      "Cache-Control": "no-store",
    });
    res.end(req.method === "HEAD" ? undefined : buf);
  } catch (e) {
    log(`handler error: ${e.message}`);
    if (!res.headersSent)
      sendJson(res, 500, { error: { code: "PROXY_ERROR", message: "internal proxy error" } });
    else res.end();
  }
});

server.listen(PORT, () => {
  const mintMode = SERVICE_SECRET ? "RS256(identity)" : "NONE";
  log(`listening on :${PORT} -> ${SDD_UPSTREAM} | auth-check ${PACA_AUTH_URL} | mint ${mintMode}`);
});
