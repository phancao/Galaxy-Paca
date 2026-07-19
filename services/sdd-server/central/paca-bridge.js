/**
 * @file Paca bridge — emits SDD sensor signals into Paca (the team task system).
 *
 * ADR-038 T6 ("SDD converges, telemetry stays outside"): the human task board
 * moves to Paca; this server stays a telemetry sensor and EMITS signals into
 * Paca as task comments:
 *
 *   Signal 1 — open `shared_core_parallel` conflicts: comment on every
 *     matching non-done Paca task (`task.custom_fields.repo` matches the
 *     conflict's repo).
 *   Signal 2 — newly recorded spec versions: comment on tasks whose
 *     `task.custom_fields.spec_doc_id` equals the published doc_id.
 *
 * Paca REST facts (Galaxy-Paca docs/api/http-design.md + task-activity.md):
 *   - envelope `{success, data, request_id}`; auth header `X-API-Key`
 *   - projects:   GET  /api/v1/projects?page=&page_size=      → data.items
 *   - statuses:   GET  /api/v1/projects/:pid/task-statuses    → data.items
 *                 (category: backlog|refinement|ready|todo|inprogress|done)
 *   - tasks:      GET  /api/v1/projects/:pid/tasks?page=&page_size=
 *                 → data.items (custom_fields JSONB; NO server-side filtering
 *                 by custom field — matching happens here, client-side)
 *   - comment:    POST /api/v1/projects/:pid/tasks/:tid/activities/comments
 *                 body {text} → 201
 *
 * Safety contract: the bridge must NEVER fail or slow ingest. Everything is
 * fire-and-forget behind try/catch; `nudge()` is synchronous, never throws and
 * is never awaited by callers. A periodic sweep (PACA_BRIDGE_POLL_MS) is the
 * source of truth — nudges only bring the next sweep forward. Every Paca call
 * has a 10s timeout and retries once on 5xx. The API key is only ever placed
 * in the X-API-Key header — never in URLs and never logged.
 *
 * Bookkeeping: `bridged_at` on sdd_conflicts / sdd_spec_versions (schema.sql).
 * A row is marked bridged only after every comment for it succeeded, so
 * failures are retried on the next sweep. Repeated conflicts on the same
 * conflict_key within SUPPRESS_WINDOW_MIN are marked bridged WITHOUT
 * re-commenting (ingest inserts a fresh conflict row per event burst).
 */

const DEFAULT_POLL_MS = 30000;
const PACA_TIMEOUT_MS = 10000;
const PROJECT_CACHE_MS = 5 * 60 * 1000;
const PAGE_SIZE = 100; // Paca list endpoints cap page_size at 100
const MAX_PAGES = 50; // hard ceiling per listing — bridge is best-effort
const BATCH_LIMIT = 20; // unbridged rows processed per sweep, oldest first
const SUPPRESS_WINDOW_MIN = 30; // mirrors the shared-core conflict window

// ── Config ──────────────────────────────────────────────────────────────────
function bridgeConfigFromEnv(env = process.env) {
  return {
    enabled: /^(1|true|on|yes)$/i.test(String(env.PACA_BRIDGE_ENABLED || "")),
    baseUrl: String(env.PACA_BASE_URL || "").replace(/\/+$/, ""),
    apiKey: String(env.PACA_API_KEY || ""),
    pollMs: parseInt(env.PACA_BRIDGE_POLL_MS, 10) > 0
      ? parseInt(env.PACA_BRIDGE_POLL_MS, 10)
      : DEFAULT_POLL_MS,
  };
}

// ── Pure matching / formatting helpers (unit-tested) ────────────────────────
function repoOfTask(task) {
  const cf = task && task.custom_fields;
  const repo = cf && typeof cf === "object" ? cf.repo : null;
  return typeof repo === "string" && repo.trim() ? repo.trim() : null;
}

/**
 * conflict_key is built by ingest as `${repo}:${dir}` (see index.js). Match by
 * prefix instead of splitting on ":" — repos themselves may contain ":" (SSH
 * remotes like `git@host:org/repo.git`), which makes splitting ambiguous.
 */
function taskMatchesConflict(task, conflictKey) {
  if (!conflictKey || typeof conflictKey !== "string") return false;
  const repo = repoOfTask(task);
  if (!repo) return false;
  return conflictKey === repo || conflictKey.startsWith(repo + ":");
}

function taskMatchesSpec(task, docId) {
  if (!docId || typeof docId !== "string") return false;
  const cf = task && task.custom_fields;
  const v = cf && typeof cf === "object" ? cf.spec_doc_id : null;
  return typeof v === "string" && v === docId;
}

/** Status ids whose category is 'done' (Paca task_statuses categories). */
function doneStatusIds(statuses) {
  const done = new Set();
  for (const s of statuses || []) if (s && s.category === "done" && s.id) done.add(s.id);
  return done;
}

/** A task with no status yet is non-done by definition. */
function isNonDoneTask(task, doneIds) {
  if (!task) return false;
  return task.status_id == null || !doneIds.has(task.status_id);
}

function parseDetail(detail) {
  if (detail == null) return {};
  if (typeof detail === "string") {
    try {
      return JSON.parse(detail) || {};
    } catch {
      return {};
    }
  }
  return typeof detail === "object" ? detail : {};
}

function formatConflictComment(conflict) {
  const d = parseDetail(conflict && conflict.detail);
  const users =
    Array.isArray(d.users) && d.users.length ? d.users.join(", ") : "unknown";
  const windowMin = d.window_min != null ? d.window_min : 30;
  return (
    `⚠️ SDD sensor: shared-core parallel edit detected on ` +
    `\`${(conflict && conflict.conflict_key) || "unknown"}\` — users: ${users}, ` +
    `window ${windowMin}m. Review before merging.`
  );
}

function formatSpecComment(spec) {
  const title = (spec && (spec.title || spec.doc_id)) || "unknown";
  const version = (spec && spec.version) || "?";
  const source = (spec && spec.source) || "hook";
  return `📄 Spec \`${title}\` version ${version} published (source: ${source}).`;
}

// ── Bookkeeping SQL (exported so tests pin the dedup query shape) ───────────
const SQL = {
  unbridgedConflicts:
    `SELECT id, kind, conflict_key, spec_doc_id, detail, created_at
       FROM sdd_conflicts
      WHERE kind = 'shared_core_parallel' AND status = 'open' AND bridged_at IS NULL
      ORDER BY id ASC LIMIT ${BATCH_LIMIT}`,
  markConflictBridged:
    "UPDATE sdd_conflicts SET bridged_at = now(), updated_at = now() WHERE id = $1 AND bridged_at IS NULL",
  recentlyBridgedSameKey:
    `SELECT 1 FROM sdd_conflicts
      WHERE conflict_key = $1 AND id <> $2 AND bridged_at IS NOT NULL
        AND bridged_at > now() - interval '${SUPPRESS_WINDOW_MIN} minutes'
      LIMIT 1`,
  unbridgedSpecVersions:
    `SELECT id, doc_id, version, title, source, published_at
       FROM sdd_spec_versions
      WHERE bridged_at IS NULL
      ORDER BY id ASC LIMIT ${BATCH_LIMIT}`,
  markSpecVersionBridged:
    "UPDATE sdd_spec_versions SET bridged_at = now() WHERE id = $1 AND bridged_at IS NULL",
};

// ── Paca REST client (X-API-Key, 10s timeout, retry-once on 5xx) ────────────
function createPacaClient({ baseUrl, apiKey, fetchImpl, timeoutMs = PACA_TIMEOUT_MS }) {
  const doFetch = fetchImpl || globalThis.fetch;

  async function call(method, apiPath, body) {
    for (let attempt = 0; ; attempt++) {
      const ctrl = new AbortController();
      const timer = setTimeout(() => ctrl.abort(), timeoutMs);
      let res;
      try {
        res = await doFetch(baseUrl + apiPath, {
          method,
          headers: {
            "X-API-Key": apiKey,
            ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
          },
          body: body !== undefined ? JSON.stringify(body) : undefined,
          signal: ctrl.signal,
        });
      } finally {
        clearTimeout(timer);
      }
      if (res.status >= 500 && attempt === 0) continue; // retry once on 5xx
      if (!res.ok) throw new Error(`Paca ${method} ${apiPath} -> HTTP ${res.status}`);
      const json = await res.json().catch(() => null); // 204 / empty body
      if (json && json.success === false)
        throw new Error(`Paca ${method} ${apiPath} -> success:false`);
      return json && json.data !== undefined ? json.data : json;
    }
  }

  /** Paginated list — Paca shape {items, total, page, page_size}. */
  async function listPaged(apiPathBase) {
    const sep = apiPathBase.includes("?") ? "&" : "?";
    const items = [];
    for (let page = 1; page <= MAX_PAGES; page++) {
      const data = await call("GET", `${apiPathBase}${sep}page=${page}&page_size=${PAGE_SIZE}`);
      const batch = (data && data.items) || (Array.isArray(data) ? data : []);
      items.push(...batch);
      const total = data && typeof data.total === "number" ? data.total : null;
      if (batch.length < PAGE_SIZE || (total != null && items.length >= total)) break;
    }
    return items;
  }

  return {
    listProjects: () => listPaged("/api/v1/projects"),
    listTaskStatuses: (projectId) =>
      call("GET", `/api/v1/projects/${projectId}/task-statuses`).then(
        (d) => (d && d.items) || (Array.isArray(d) ? d : [])
      ),
    listTasks: (projectId) => listPaged(`/api/v1/projects/${projectId}/tasks`),
    postTaskComment: (projectId, taskId, text) =>
      call("POST", `/api/v1/projects/${projectId}/tasks/${taskId}/activities/comments`, { text }),
  };
}

// ── Bridge ──────────────────────────────────────────────────────────────────
/**
 * Start the bridge. Returns a handle:
 *   { enabled, nudge(), sweep(), stop() }
 * `nudge()` never throws — call it fire-and-forget from ingest right after
 * inserting a conflict / spec version. When disabled (default) the handle is
 * inert, so callers never need to branch.
 */
function startPacaBridge({ db, log = console, env = process.env, fetchImpl, config } = {}) {
  const cfg = config || bridgeConfigFromEnv(env);
  const inert = { enabled: false, nudge() {}, sweep: async () => {}, stop() {} };
  if (!cfg.enabled) return inert;
  if (!cfg.baseUrl || !cfg.apiKey) {
    log.warn(
      "[PACA-BRIDGE] PACA_BRIDGE_ENABLED is set but PACA_BASE_URL / PACA_API_KEY are missing — bridge stays inert"
    );
    return inert;
  }

  const paca = createPacaClient({ baseUrl: cfg.baseUrl, apiKey: cfg.apiKey, fetchImpl });
  let projectCache = { at: 0, projects: null };
  let running = false;
  let again = false;
  let stopped = false;

  async function getProjects() {
    if (projectCache.projects && Date.now() - projectCache.at < PROJECT_CACHE_MS)
      return projectCache.projects;
    const projects = await paca.listProjects();
    projectCache = { at: Date.now(), projects };
    return projects;
  }

  /**
   * Snapshot of non-done tasks across all visible Paca projects. All-or-
   * nothing: a partial snapshot must not mark rows bridged (it could miss
   * matches forever), so any listing failure aborts the whole sweep.
   */
  async function loadTaskSnapshot() {
    const projects = await getProjects();
    const snapshot = [];
    for (const p of projects || []) {
      const statuses = await paca.listTaskStatuses(p.id);
      const done = doneStatusIds(statuses);
      const tasks = (await paca.listTasks(p.id)).filter((t) => isNonDoneTask(t, done));
      snapshot.push({ projectId: p.id, tasks });
    }
    return snapshot;
  }

  /** Post one comment to every matching task; true = safe to mark bridged. */
  async function commentMatches(snapshot, matcher, text, label) {
    let posted = 0;
    let failed = 0;
    for (const { projectId, tasks } of snapshot) {
      for (const t of tasks) {
        if (!matcher(t)) continue;
        try {
          await paca.postTaskComment(projectId, t.id, text);
          posted++;
        } catch (err) {
          failed++;
          log.warn(`[PACA-BRIDGE] comment on task ${t.id} failed (${label}): ${err.message}`);
        }
      }
    }
    if (posted) log.log(`[PACA-BRIDGE] ${label}: ${posted} comment(s) posted`);
    return failed === 0;
  }

  async function runSweep() {
    const conflicts = (await db.q(SQL.unbridgedConflicts)).rows;
    const specs = (await db.q(SQL.unbridgedSpecVersions)).rows;
    if (!conflicts.length && !specs.length) return; // nothing to do — don't touch Paca

    const snapshot = await loadTaskSnapshot(); // throws → retried next sweep

    for (const c of conflicts) {
      // Ingest inserts a fresh conflict row per event burst; if the same
      // conflict_key was bridged within the window, mark without re-commenting.
      const dup = await db.q(SQL.recentlyBridgedSameKey, [c.conflict_key, c.id]);
      if (dup.rows.length) {
        await db.q(SQL.markConflictBridged, [c.id]);
        continue;
      }
      const ok = await commentMatches(
        snapshot,
        (t) => taskMatchesConflict(t, c.conflict_key),
        formatConflictComment(c),
        `conflict #${c.id} ${c.conflict_key}`
      );
      if (ok) await db.q(SQL.markConflictBridged, [c.id]);
    }

    for (const s of specs) {
      const ok = await commentMatches(
        snapshot,
        (t) => taskMatchesSpec(t, s.doc_id),
        formatSpecComment(s),
        `spec ${s.doc_id}@${s.version}`
      );
      if (ok) await db.q(SQL.markSpecVersionBridged, [s.id]);
    }
  }

  /** Serialized, never-throwing sweep entry point. */
  async function sweep() {
    if (stopped) return;
    if (running) {
      again = true; // coalesce: one more pass right after the current one
      return;
    }
    running = true;
    try {
      await runSweep();
    } catch (err) {
      log.warn(`[PACA-BRIDGE] sweep failed (will retry): ${err.message}`);
    } finally {
      running = false;
      if (again && !stopped) {
        again = false;
        setImmediate(() => sweep());
      }
    }
  }

  function nudge() {
    if (stopped) return;
    try {
      setImmediate(() => sweep());
    } catch {
      /* never throw into ingest */
    }
  }

  const timer = setInterval(() => sweep(), cfg.pollMs);
  if (timer.unref) timer.unref();
  const kickoff = setTimeout(() => sweep(), 3000); // boot catch-up
  if (kickoff.unref) kickoff.unref();
  log.log(`[PACA-BRIDGE] enabled — ${cfg.baseUrl}, poll ${cfg.pollMs}ms`);

  return {
    enabled: true,
    nudge,
    sweep,
    stop() {
      stopped = true;
      clearInterval(timer);
      clearTimeout(kickoff);
    },
  };
}

module.exports = {
  startPacaBridge,
  createPacaClient,
  bridgeConfigFromEnv,
  // pure helpers + query shapes (exported for unit tests)
  repoOfTask,
  taskMatchesConflict,
  taskMatchesSpec,
  doneStatusIds,
  isNonDoneTask,
  formatConflictComment,
  formatSpecComment,
  SQL,
  BATCH_LIMIT,
  SUPPRESS_WINDOW_MIN,
};
