/**
 * @file Authentication for the Galaxy SDD Coordination Server.
 *
 * ONE signature algorithm: RS256, verified against the Vortex JWKS. Every
 * token this server accepts is one the identity service minted with a private
 * key it alone holds.
 *
 *   - INGEST (/api/ingest): a Vortex *delegation token* — RS256 with
 *     token_type="delegation". Dev machines' sdd-agents carry it. Gating on
 *     token_type is what keeps ingest agent-only: a plain OIDC access token
 *     (an interactive user) can read but never write.
 *   - READ API + UI: a Vortex OIDC *access token* — RS256, obtained by the
 *     browser via authorization_code + PKCE. Means "a logged-in Vortex user".
 *
 * HS256 IS NOT ACCEPTED, and the reason is specific rather than hygienic.
 * `JWT_SECRET` is one value shared across the whole Galaxy fleet, so the key
 * this server used to VERIFY a delegation token was a key a dozen unrelated
 * containers could SIGN one with. Ingest writes the fleet telemetry every
 * dashboard reads; "can verify" and "can forge the record" were the same
 * permission. Under RS256 only identity can mint, and JWKS lets everyone else
 * check without ever holding a signing key.
 */

const jwt = require("jsonwebtoken");
const crypto = require("crypto");

// ── RS256 (OIDC access token / delegation token) via Vortex JWKS ────────────
// JWKS lives behind the identity service. Inside galaxy_network the server can
// reach it directly; configurable for other topologies.
const JWKS_URL = process.env.NEXUS_JWKS_URL || "http://vortex-identity:8086/.well-known/jwks.json";
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
/** INGEST: an RS256 delegation token only. */
async function requireAuth(req, res, next) {
  const token = bearer(req.headers.authorization);
  // Gate on token_type=delegation so a plain OIDC access token (an interactive
  // user) can never ingest — ingest stays agent-only.
  const rs = token ? await verifyRs(token) : null;
  const payload = rs && rs.token_type === "delegation" ? rs : null;
  if (!payload || !payload.sub) {
    return res
      .status(401)
      .json({ error: { code: "UNAUTHENTICATED", message: "valid delegation token required" } });
  }
  req.actor = actorFrom(payload, payload.token_type || "delegation");
  next();
}

/** READ: a logged-in identity — an RS256 OIDC access token. */
async function requireRead(req, res, next) {
  const token = bearer(req.headers.authorization);
  if (!token)
    return res.status(401).json({ error: { code: "UNAUTHENTICATED", message: "login required" } });
  const payload = await verifyRs(token);
  if (!payload || !payload.sub) {
    return res
      .status(401)
      .json({ error: { code: "UNAUTHENTICATED", message: "invalid or expired token" } });
  }
  req.actor = actorFrom(payload);
  next();
}

module.exports = { requireAuth, requireRead, verifyRs };
