/**
 * Negative tests for the SDD Coordination Server auth surface.
 *
 * These FAIL against the code as it stood before HS256 was removed. That is the
 * point — a control nobody has watched fail is not a control. Both runs are
 * quoted in the PR.
 *
 * The defect: `JWT_SECRET` is a single value shared across the Galaxy fleet, so
 * the key `verifyHs` used to VERIFY an ingest delegation token was a key many
 * unrelated containers could SIGN one with. `/api/ingest` writes the fleet
 * telemetry every dashboard reads, so "may verify" and "may forge the record"
 * were the same permission. `requireRead` accepted HS256 too, which meant the
 * same shared secret also minted a read identity.
 *
 * Run: node --test __tests__/auth-no-hs256.test.js
 */
"use strict";

const test = require("node:test");
const assert = require("node:assert");
const jwt = require("jsonwebtoken");

const SHARED = "pretend-this-is-the-fleet-JWT_SECRET-0123456789";

process.env.JWT_SECRET = SHARED;
process.env.NEXUS_JWT_SECRET = SHARED;
// Point JWKS at an address nothing answers on, so RS256 verification cannot
// accidentally succeed and mask an HS256 acceptance.
process.env.NEXUS_JWKS_URL = "http://127.0.0.1:1/.well-known/jwks.json";

const auth = require("../auth");

function hs256(claims) {
  return jwt.sign({ sub: "agent-1", ...claims }, SHARED, { algorithm: "HS256", expiresIn: "15m" });
}

function fakeRes() {
  return {
    statusCode: null,
    body: null,
    status(c) {
      this.statusCode = c;
      return this;
    },
    json(b) {
      this.body = b;
      return this;
    },
  };
}

async function run(mw, token) {
  const req = { headers: { authorization: `Bearer ${token}` } };
  const res = fakeRes();
  let passed = false;
  await mw(req, res, () => {
    passed = true;
  });
  return { passed, res, req };
}

test("ingest rejects an HS256 delegation token signed with the shared secret", async () => {
  const { passed, res } = await run(auth.requireAuth, hs256({ token_type: "delegation" }));
  assert.strictEqual(passed, false, "HS256 delegation token was ACCEPTED for ingest");
  assert.strictEqual(res.statusCode, 401);
});

test("read rejects an HS256 session token signed with the shared secret", async () => {
  const { passed, res } = await run(auth.requireRead, hs256({ token_type: "session" }));
  assert.strictEqual(passed, false, "HS256 session token was ACCEPTED for read");
  assert.strictEqual(res.statusCode, 401);
});

test("the HS256 verifier is not merely bypassed — it no longer exists", () => {
  // A disabled branch is how this comes back. The export must be gone, not
  // just unreferenced by the two middlewares.
  assert.strictEqual(
    typeof auth.verifyHs,
    "undefined",
    "verifyHs is still exported; the HS256 path was disabled rather than deleted"
  );
});

test("control: an unsigned alg:none token is rejected (passes before and after)", async () => {
  const none = jwt.sign({ sub: "agent-1", token_type: "delegation" }, "", { algorithm: "none" });
  const { passed } = await run(auth.requireAuth, none);
  assert.strictEqual(passed, false, "alg:none accepted");
});
