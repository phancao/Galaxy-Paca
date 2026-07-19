/**
 * @file Authentication for the Galaxy SDD Coordination Server.
 *
 * Two token classes, two purposes:
 *   - INGEST (/api/ingest): a Vortex *delegation token* — JWT HS256 signed with
 *     the shared Galaxy JWT_SECRET (token_type="delegation"). Dev machines'
 *     sdd-agents carry it. Verified by shared secret (+ rotation).
 *   - READ API + UI: a Vortex OIDC *access token* — RS256, signed by the Vortex
 *     private key, obtained by the browser via authorization_code + PKCE. Means
 *     "a logged-in Vortex user". Verified against the Vortex JWKS.
 *
 * `verifyAny` accepts either, so an authenticated identity (machine OR human)
 * can read; only the agent's HS256 delegation token may ingest.
 */

const jwt = require("jsonwebtoken");
const crypto = require("crypto");

// ── HS256 (delegation / session) ────────────────────────────────────────────
function hsSecrets() {
  const cur = process.env.NEXUS_JWT_SECRET || process.env.JWT_SECRET || "";
  const prev = process.env.JWT_SECRET_PREVIOUS || process.env.NEXUS_JWT_SECRET_PREVIOUS || "";
  return [cur, prev].filter(Boolean);
}

function verifyHs(token) {
  for (const key of hsSecrets()) {
    try {
      return jwt.verify(token, key, { algorithms: ["HS256"] });
    } catch {
      /* try next */
    }
  }
  return null;
}

// ── RS256 (OIDC access token) via Vortex JWKS ────────────────────────────────
// JWKS lives behind the identity service. Inside galaxy_network the server can
// reach it directly; configurable for other topologies.
const JWKS_URL = process.env.NEXUS_JWKS_URL || "http://nexus-identity:8086/.well-known/jwks.json";
let jwksCache = { keys: {}, fetchedAt: 0 };

async function loadJwks(force = false) {
  const fresh = Date.now() - jwksCache.fetchedAt < 10 * 60 * 1000;
  if (!force && fresh && Object.keys(jwksCache.keys).length) return jwksCache.keys;
  try {
    const res = await fetch(JWKS_URL);
    const body = await res.json();
    const keys = {};
    for (const jwk of body.keys || []) {
      try {
        const pem = crypto
          .createPublicKey({ key: jwk, format: "jwk" })
          .export({ type: "spki", format: "pem" });
        keys[jwk.kid] = pem;
      } catch {
        /* skip bad key */
      }
    }
    jwksCache = { keys, fetchedAt: Date.now() };
  } catch (err) {
    console.warn("[AUTH] JWKS fetch failed:", err.message);
  }
  return jwksCache.keys;
}

async function verifyRs(token) {
  let header;
  try {
    header = JSON.parse(Buffer.from(token.split(".")[0], "base64url").toString());
  } catch {
    return null;
  }
  let keys = await loadJwks();
  let pem = keys[header.kid];
  if (!pem) {
    keys = await loadJwks(true); // unknown kid → refresh once (key rotation)
    pem = keys[header.kid];
  }
  if (!pem) return null;
  try {
    return jwt.verify(token, pem, { algorithms: ["RS256"] });
  } catch {
    return null;
  }
}

function bearer(authHeader) {
  const m = /^Bearer\s+(.+)$/i.exec(authHeader || "");
  return m ? m[1].trim() : null;
}

function actorFrom(payload, tokenType) {
  return {
    sub: String(payload.sub),
    email: payload.email || null,
    name: payload.name || null,
    tokenType: tokenType || payload.token_type || "session",
  };
}

// ── Middleware ──────────────────────────────────────────────────────────────
/** INGEST: delegation (HS256) only. */
async function requireAuth(req, res, next) {
  const token = bearer(req.headers.authorization);
  let payload = token && verifyHs(token);
  // §11 dual-accept: also honor an RS256-signed *delegation* token during the
  // HS256->RS256 migration. Gate on token_type=delegation so a plain OIDC
  // access token (an interactive user) can never ingest — ingest stays agent-only.
  if (!payload && token) {
    const rs = await verifyRs(token);
    if (rs && rs.token_type === "delegation") payload = rs;
  }
  if (!payload || !payload.sub) {
    return res
      .status(401)
      .json({ error: { code: "UNAUTHENTICATED", message: "valid delegation token required" } });
  }
  req.actor = actorFrom(payload, payload.token_type || "delegation");
  next();
}

/** READ: a logged-in identity — OIDC access token (RS256) OR HS256. */
async function requireRead(req, res, next) {
  const token = bearer(req.headers.authorization);
  if (!token)
    return res.status(401).json({ error: { code: "UNAUTHENTICATED", message: "login required" } });
  let payload = verifyHs(token);
  if (!payload) payload = await verifyRs(token);
  if (!payload || !payload.sub) {
    return res
      .status(401)
      .json({ error: { code: "UNAUTHENTICATED", message: "invalid or expired token" } });
  }
  req.actor = actorFrom(payload);
  next();
}

module.exports = { requireAuth, requireRead, verifyHs, verifyRs };
