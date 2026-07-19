/**
 * @file Unit tests for the Paca bridge (central/paca-bridge.js) — pure
 * matching/formatting helpers, dedup bookkeeping query shape, the REST client
 * (mocked fetch: X-API-Key, pagination, envelope, retry-once-on-5xx) and a
 * full mocked sweep. No network, no Postgres.
 */

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");

const {
  bridgeConfigFromEnv,
  repoOfTask,
  taskMatchesConflict,
  taskMatchesSpec,
  doneStatusIds,
  isNonDoneTask,
  formatConflictComment,
  formatSpecComment,
  SQL,
  BATCH_LIMIT,
  createPacaClient,
  startPacaBridge,
} = require("../paca-bridge");

// ── Config ──────────────────────────────────────────────────────────────────
describe("bridgeConfigFromEnv", () => {
  it("is disabled by default", () => {
    assert.equal(bridgeConfigFromEnv({}).enabled, false);
  });
  it("accepts 1/true/on/yes (case-insensitive), rejects everything else", () => {
    for (const v of ["1", "true", "TRUE", "on", "yes"])
      assert.equal(bridgeConfigFromEnv({ PACA_BRIDGE_ENABLED: v }).enabled, true, v);
    for (const v of ["0", "false", "off", "", "enabled"])
      assert.equal(bridgeConfigFromEnv({ PACA_BRIDGE_ENABLED: v }).enabled, false, v);
  });
  it("defaults poll to 30000ms and rejects nonsense values", () => {
    assert.equal(bridgeConfigFromEnv({}).pollMs, 30000);
    assert.equal(bridgeConfigFromEnv({ PACA_BRIDGE_POLL_MS: "5000" }).pollMs, 5000);
    assert.equal(bridgeConfigFromEnv({ PACA_BRIDGE_POLL_MS: "-1" }).pollMs, 30000);
    assert.equal(bridgeConfigFromEnv({ PACA_BRIDGE_POLL_MS: "abc" }).pollMs, 30000);
  });
  it("trims trailing slashes off the base URL", () => {
    assert.equal(
      bridgeConfigFromEnv({ PACA_BASE_URL: "http://paca-gateway:80///" }).baseUrl,
      "http://paca-gateway:80"
    );
  });
});

// ── Repo matching (Signal 1) ────────────────────────────────────────────────
describe("taskMatchesConflict", () => {
  const task = (repo) => ({ id: "t1", custom_fields: repo == null ? {} : { repo } });

  it("matches when conflict_key starts with `<repo>:`", () => {
    assert.equal(taskMatchesConflict(task("myrepo"), "myrepo:/src/core"), true);
  });
  it("matches an exact repo-only key", () => {
    assert.equal(taskMatchesConflict(task("myrepo"), "myrepo"), true);
  });
  it("handles repos that themselves contain ':' (SSH remotes)", () => {
    const repo = "git@github.com:org/repo.git";
    assert.equal(taskMatchesConflict(task(repo), `${repo}:/packages/core`), true);
  });
  it("does not match a different repo, nor a bare prefix without ':' boundary", () => {
    assert.equal(taskMatchesConflict(task("other"), "myrepo:/src/core"), false);
    assert.equal(taskMatchesConflict(task("my"), "myrepo:/src/core"), false);
  });
  it("requires a non-empty string custom_fields.repo", () => {
    assert.equal(taskMatchesConflict(task(null), "myrepo:/src"), false);
    assert.equal(taskMatchesConflict({ id: "t" }, "myrepo:/src"), false);
    assert.equal(taskMatchesConflict(task("   "), "myrepo:/src"), false);
    assert.equal(taskMatchesConflict(task(42), "42:/src"), false);
  });
  it("requires a conflict key", () => {
    assert.equal(taskMatchesConflict(task("myrepo"), null), false);
    assert.equal(taskMatchesConflict(task("myrepo"), ""), false);
  });
  it("repoOfTask trims whitespace", () => {
    assert.equal(repoOfTask({ custom_fields: { repo: "  r  " } }), "r");
  });
});

// ── Spec matching (Signal 2) ────────────────────────────────────────────────
describe("taskMatchesSpec", () => {
  it("matches on exact custom_fields.spec_doc_id", () => {
    assert.equal(taskMatchesSpec({ custom_fields: { spec_doc_id: "doc-1" } }, "doc-1"), true);
  });
  it("rejects mismatches, missing fields and non-strings", () => {
    assert.equal(taskMatchesSpec({ custom_fields: { spec_doc_id: "doc-2" } }, "doc-1"), false);
    assert.equal(taskMatchesSpec({ custom_fields: {} }, "doc-1"), false);
    assert.equal(taskMatchesSpec({}, "doc-1"), false);
    assert.equal(taskMatchesSpec({ custom_fields: { spec_doc_id: 7 } }, "7"), false);
    assert.equal(taskMatchesSpec({ custom_fields: { spec_doc_id: "doc-1" } }, null), false);
  });
});

// ── Non-done statuses ───────────────────────────────────────────────────────
describe("doneStatusIds / isNonDoneTask", () => {
  const statuses = [
    { id: "s-todo", category: "todo" },
    { id: "s-prog", category: "inprogress" },
    { id: "s-done", category: "done" },
  ];
  it("collects only category='done' status ids", () => {
    assert.deepEqual([...doneStatusIds(statuses)], ["s-done"]);
    assert.equal(doneStatusIds(null).size, 0);
  });
  it("treats null status as non-done and done statuses as done", () => {
    const done = doneStatusIds(statuses);
    assert.equal(isNonDoneTask({ status_id: "s-todo" }, done), true);
    assert.equal(isNonDoneTask({ status_id: null }, done), true);
    assert.equal(isNonDoneTask({}, done), true);
    assert.equal(isNonDoneTask({ status_id: "s-done" }, done), false);
  });
});

// ── Comment formatting ──────────────────────────────────────────────────────
describe("comment formatting", () => {
  it("formats the conflict comment with users + window", () => {
    const msg = formatConflictComment({
      conflict_key: "myrepo:/src/core",
      detail: { users: ["u1", "u2"], where: "myrepo:/src/core", window_min: 30 },
    });
    assert.equal(
      msg,
      "⚠️ SDD sensor: shared-core parallel edit detected on `myrepo:/src/core` — users: u1, u2, window 30m. Review before merging."
    );
  });
  it("parses stringified detail and falls back on missing fields", () => {
    const msg = formatConflictComment({
      conflict_key: "r:/d",
      detail: JSON.stringify({ users: ["a"], window_min: 15 }),
    });
    assert.match(msg, /users: a, window 15m/);
    const bare = formatConflictComment({ conflict_key: "r:/d", detail: null });
    assert.match(bare, /users: unknown, window 30m/);
  });
  it("formats the spec comment, falling back to doc_id when title is null", () => {
    assert.equal(
      formatSpecComment({ doc_id: "d1", version: "3", title: "Auth Spec", source: "hook" }),
      "📄 Spec `Auth Spec` version 3 published (source: hook)."
    );
    assert.equal(
      formatSpecComment({ doc_id: "d1", version: "3", title: null, source: null }),
      "📄 Spec `d1` version 3 published (source: hook)."
    );
  });
});

// ── Dedup bookkeeping query shape ───────────────────────────────────────────
describe("bookkeeping SQL shape", () => {
  it("selects only open, unbridged shared_core_parallel conflicts, oldest first, bounded", () => {
    const q = SQL.unbridgedConflicts;
    assert.match(q, /FROM sdd_conflicts/);
    assert.match(q, /kind = 'shared_core_parallel'/);
    assert.match(q, /status = 'open'/);
    assert.match(q, /bridged_at IS NULL/);
    assert.match(q, /ORDER BY id ASC/);
    assert.match(q, new RegExp(`LIMIT ${BATCH_LIMIT}`));
  });
  it("selects only unbridged spec versions, oldest first, bounded", () => {
    const q = SQL.unbridgedSpecVersions;
    assert.match(q, /FROM sdd_spec_versions/);
    assert.match(q, /bridged_at IS NULL/);
    assert.match(q, /ORDER BY id ASC/);
    assert.match(q, new RegExp(`LIMIT ${BATCH_LIMIT}`));
  });
  it("mark statements set bridged_at once and guard against double-marking", () => {
    assert.match(SQL.markConflictBridged, /UPDATE sdd_conflicts SET bridged_at = now\(\)/);
    assert.match(SQL.markConflictBridged, /WHERE id = \$1 AND bridged_at IS NULL/);
    assert.match(SQL.markSpecVersionBridged, /UPDATE sdd_spec_versions SET bridged_at = now\(\)/);
    assert.match(SQL.markSpecVersionBridged, /WHERE id = \$1 AND bridged_at IS NULL/);
  });
  it("same-key suppression looks at other rows bridged inside the window", () => {
    const q = SQL.recentlyBridgedSameKey;
    assert.match(q, /conflict_key = \$1/);
    assert.match(q, /id <> \$2/);
    assert.match(q, /bridged_at IS NOT NULL/);
    assert.match(q, /interval '30 minutes'/);
  });
});

// ── Paca client with mocked fetch ───────────────────────────────────────────
function jsonRes(data, { status = 200, envelope = true } = {}) {
  const body = envelope ? { success: true, data, request_id: "req-1" } : data;
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  };
}

describe("createPacaClient (mocked fetch)", () => {
  it("sends X-API-Key, paginates task lists and unwraps the envelope", async () => {
    const calls = [];
    const page1 = { items: Array.from({ length: 100 }, (_, i) => ({ id: `t${i}` })), total: 130, page: 1, page_size: 100 };
    const page2 = { items: Array.from({ length: 30 }, (_, i) => ({ id: `t${100 + i}` })), total: 130, page: 2, page_size: 100 };
    const fetchImpl = async (url, opts) => {
      calls.push({ url, opts });
      return jsonRes(url.includes("page=2") ? page2 : page1);
    };
    const paca = createPacaClient({ baseUrl: "http://paca", apiKey: "sekret", fetchImpl });
    const tasks = await paca.listTasks("p1");
    assert.equal(tasks.length, 130);
    assert.equal(calls.length, 2);
    assert.equal(calls[0].opts.headers["X-API-Key"], "sekret");
    assert.match(calls[0].url, /^http:\/\/paca\/api\/v1\/projects\/p1\/tasks\?page=1&page_size=100$/);
    assert.match(calls[1].url, /page=2&page_size=100$/);
  });

  it("stops after one short page", async () => {
    let n = 0;
    const fetchImpl = async () => (n++, jsonRes({ items: [{ id: "p" }], total: 1 }));
    const paca = createPacaClient({ baseUrl: "http://paca", apiKey: "k", fetchImpl });
    assert.equal((await paca.listProjects()).length, 1);
    assert.equal(n, 1);
  });

  it("retries exactly once on 5xx, then succeeds", async () => {
    let n = 0;
    const fetchImpl = async () =>
      ++n === 1 ? jsonRes({ error: "boom" }, { status: 502, envelope: false }) : jsonRes({ id: "c1" });
    const paca = createPacaClient({ baseUrl: "http://paca", apiKey: "k", fetchImpl });
    const out = await paca.postTaskComment("p1", "t1", "hello");
    assert.equal(out.id, "c1");
    assert.equal(n, 2);
  });

  it("fails after the second consecutive 5xx and on 4xx without retry", async () => {
    let n5 = 0;
    const always502 = async () => (n5++, jsonRes({}, { status: 502, envelope: false }));
    const paca5 = createPacaClient({ baseUrl: "http://paca", apiKey: "k", fetchImpl: always502 });
    await assert.rejects(() => paca5.postTaskComment("p", "t", "x"), /HTTP 502/);
    assert.equal(n5, 2);

    let n4 = 0;
    const always404 = async () => (n4++, jsonRes({}, { status: 404, envelope: false }));
    const paca4 = createPacaClient({ baseUrl: "http://paca", apiKey: "k", fetchImpl: always404 });
    await assert.rejects(() => paca4.listTaskStatuses("p"), /HTTP 404/);
    assert.equal(n4, 1);
  });

  it("posts comments with a JSON {text} body", async () => {
    let seen;
    const fetchImpl = async (url, opts) => ((seen = { url, opts }), jsonRes({ id: "c" }, { status: 201 }));
    const paca = createPacaClient({ baseUrl: "http://paca", apiKey: "k", fetchImpl });
    await paca.postTaskComment("p1", "t9", "msg");
    assert.equal(seen.url, "http://paca/api/v1/projects/p1/tasks/t9/activities/comments");
    assert.equal(seen.opts.method, "POST");
    assert.deepEqual(JSON.parse(seen.opts.body), { text: "msg" });
    assert.equal(seen.opts.headers["Content-Type"], "application/json");
  });
});

// ── Full sweep with mocked fetch + fake db ──────────────────────────────────
describe("startPacaBridge sweep (mocked fetch + fake db)", () => {
  function fakePaca() {
    // One project, three tasks: repo match (non-done), repo match (done), spec match.
    const routes = [
      [/\/api\/v1\/projects\?/, { items: [{ id: "p1", name: "Proj" }], total: 1 }],
      [
        /\/projects\/p1\/task-statuses$/,
        { items: [{ id: "s-open", category: "todo" }, { id: "s-done", category: "done" }] },
      ],
      [
        /\/projects\/p1\/tasks\?/,
        {
          items: [
            { id: "t-repo", status_id: "s-open", custom_fields: { repo: "myrepo" } },
            { id: "t-done", status_id: "s-done", custom_fields: { repo: "myrepo" } },
            { id: "t-spec", status_id: null, custom_fields: { spec_doc_id: "doc-1" } },
          ],
          total: 3,
        },
      ],
    ];
    const comments = [];
    const fetchImpl = async (url, opts) => {
      const m = /\/tasks\/([^/]+)\/activities\/comments$/.exec(url);
      if (m) {
        comments.push({ taskId: m[1], text: JSON.parse(opts.body).text, key: opts.headers["X-API-Key"] });
        return jsonRes({ id: `c${comments.length}` }, { status: 201 });
      }
      for (const [re, data] of routes) if (re.test(url)) return jsonRes(data);
      throw new Error(`unexpected fetch: ${url}`);
    };
    return { fetchImpl, comments };
  }

  function fakeDb({ conflicts = [], specs = [] } = {}) {
    const marked = { conflicts: [], specs: [] };
    return {
      marked,
      q: async (text, params) => {
        if (text === SQL.unbridgedConflicts) return { rows: conflicts };
        if (text === SQL.unbridgedSpecVersions) return { rows: specs };
        if (text === SQL.recentlyBridgedSameKey) return { rows: [] };
        if (text === SQL.markConflictBridged) return (marked.conflicts.push(params[0]), { rows: [] });
        if (text === SQL.markSpecVersionBridged) return (marked.specs.push(params[0]), { rows: [] });
        throw new Error(`unexpected query: ${text}`);
      },
    };
  }

  const silent = { log() {}, warn() {}, error() {} };
  const cfg = { enabled: true, baseUrl: "http://paca", apiKey: "sekret", pollMs: 3600000 };

  it("comments on matching non-done tasks and marks rows bridged", async () => {
    const { fetchImpl, comments } = fakePaca();
    const db = fakeDb({
      conflicts: [
        { id: 11, kind: "shared_core_parallel", conflict_key: "myrepo:/src/core", detail: { users: ["u1", "u2"], window_min: 30 } },
      ],
      specs: [{ id: 21, doc_id: "doc-1", version: "2", title: "Spec X", source: "hook" }],
    });
    const bridge = startPacaBridge({ db, log: silent, fetchImpl, config: cfg });
    try {
      await bridge.sweep();
    } finally {
      bridge.stop();
    }
    // conflict → only t-repo (t-done is done, t-spec has no repo); spec → only t-spec
    assert.deepEqual(
      comments.map((c) => c.taskId),
      ["t-repo", "t-spec"]
    );
    assert.match(comments[0].text, /shared-core parallel edit detected on `myrepo:\/src\/core`/);
    assert.match(comments[0].text, /users: u1, u2, window 30m/);
    assert.equal(comments[1].text, "📄 Spec `Spec X` version 2 published (source: hook).");
    assert.deepEqual(db.marked, { conflicts: [11], specs: [21] });
  });

  it("marks rows bridged even with zero matching tasks (signal evaluated)", async () => {
    const { fetchImpl, comments } = fakePaca();
    const db = fakeDb({
      conflicts: [{ id: 12, conflict_key: "otherrepo:/x", detail: {} }],
    });
    const bridge = startPacaBridge({ db, log: silent, fetchImpl, config: cfg });
    try {
      await bridge.sweep();
    } finally {
      bridge.stop();
    }
    assert.equal(comments.length, 0);
    assert.deepEqual(db.marked.conflicts, [12]);
  });

  it("does NOT mark bridged when a comment post fails (retried next sweep)", async () => {
    const base = fakePaca();
    const failingFetch = async (url, opts) => {
      if (/activities\/comments$/.test(url)) return jsonRes({}, { status: 503, envelope: false });
      return base.fetchImpl(url, opts);
    };
    const db = fakeDb({
      conflicts: [{ id: 13, conflict_key: "myrepo:/src", detail: { users: ["u"] } }],
    });
    const bridge = startPacaBridge({ db, log: silent, fetchImpl: failingFetch, config: cfg });
    try {
      await bridge.sweep();
    } finally {
      bridge.stop();
    }
    assert.deepEqual(db.marked.conflicts, []);
  });

  it("skips Paca entirely when there is nothing unbridged", async () => {
    let fetches = 0;
    const fetchImpl = async () => (fetches++, jsonRes({ items: [] }));
    const db = fakeDb();
    const bridge = startPacaBridge({ db, log: silent, fetchImpl, config: cfg });
    try {
      await bridge.sweep();
    } finally {
      bridge.stop();
    }
    assert.equal(fetches, 0);
  });

  it("suppresses re-comment when the same key was bridged within the window", async () => {
    const { fetchImpl, comments } = fakePaca();
    const db = fakeDb({ conflicts: [{ id: 14, conflict_key: "myrepo:/src/core", detail: {} }] });
    const origQ = db.q;
    db.q = async (text, params) =>
      text === SQL.recentlyBridgedSameKey ? { rows: [{ 1: 1 }] } : origQ(text, params);
    const bridge = startPacaBridge({ db, log: silent, fetchImpl, config: cfg });
    try {
      await bridge.sweep();
    } finally {
      bridge.stop();
    }
    assert.equal(comments.length, 0);
    assert.deepEqual(db.marked.conflicts, [14]); // marked, but silently
  });

  it("stays inert when disabled or misconfigured", async () => {
    const db = { q: async () => { throw new Error("must not be called"); } };
    const off = startPacaBridge({ db, log: silent, env: {} });
    assert.equal(off.enabled, false);
    off.nudge(); // no-throw
    await off.sweep();
    const half = startPacaBridge({ db, log: silent, env: { PACA_BRIDGE_ENABLED: "true" } });
    assert.equal(half.enabled, false);
  });
});
