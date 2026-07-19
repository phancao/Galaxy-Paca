/**
 * @file Postgres data layer for the Galaxy SDD Coordination Server. Async pg
 * pool + the upsert/insert/query helpers the ingest path and team API need.
 * Multi-tenant: every write is tagged with the user (Vortex `sub`) + host.
 *
 * Unlike the per-machine monitor (better-sqlite3, sync), this is the central
 * online store: many machines write concurrently, so it is Postgres + async.
 */

const fs = require("fs");
const path = require("path");
const { Pool } = require("pg");

const pool = new Pool({
  connectionString: process.env.DATABASE_URL,
  // Falls back to standard PG* env vars when DATABASE_URL is unset.
  max: parseInt(process.env.PG_POOL_MAX || "10", 10),
});

pool.on("error", (err) => console.error("[DB] idle client error:", err.message));

/** Apply the schema (idempotent — all CREATE ... IF NOT EXISTS). */
async function init() {
  const sql = fs.readFileSync(path.join(__dirname, "schema.sql"), "utf8");
  await pool.query(sql);
}

const q = (text, params) => pool.query(text, params);

// ── Identity ────────────────────────────────────────────────────────────────
async function upsertUser({ sub, email, name }) {
  await q(
    `INSERT INTO users (id, email, name) VALUES ($1,$2,$3)
     ON CONFLICT (id) DO UPDATE SET
       email = COALESCE(EXCLUDED.email, users.email),
       name  = COALESCE(EXCLUDED.name, users.name),
       last_seen = now()`,
    [sub, email || null, name || null]
  );
}

async function upsertHost(userId, hostname) {
  if (!hostname) return;
  await q(
    `INSERT INTO hosts (user_id, hostname) VALUES ($1,$2)
     ON CONFLICT (user_id, hostname) DO UPDATE SET last_seen = now()`,
    [userId, hostname]
  );
}

// ── Sessions / agents / events ──────────────────────────────────────────────
async function ensureSession(sessionId, userId, hostname, data) {
  await q(
    `INSERT INTO sessions (id, user_id, hostname, status, cwd, repo, model, metadata)
     VALUES ($1,$2,$3,'active',$4,$5,$6,$7)
     ON CONFLICT (id) DO UPDATE SET
       updated_at = now(),
       hostname = COALESCE(sessions.hostname, EXCLUDED.hostname),
       cwd = COALESCE(sessions.cwd, EXCLUDED.cwd),
       repo = COALESCE(sessions.repo, EXCLUDED.repo),
       model = COALESCE(sessions.model, EXCLUDED.model)`,
    [
      sessionId,
      userId,
      hostname || null,
      (data && data.cwd) || null,
      (data && data.repo) || null,
      (data && data.model) || null,
      data ? JSON.stringify(data.session_metadata || {}) : null,
    ]
  );
}

async function touchSession(sessionId) {
  await q("UPDATE sessions SET updated_at = now() WHERE id = $1", [sessionId]);
}

async function setSessionStatus(sessionId, status, ended) {
  await q(
    `UPDATE sessions SET status=$2, updated_at=now(), ended_at = CASE WHEN $3 THEN now() ELSE ended_at END WHERE id=$1`,
    [sessionId, status, !!ended]
  );
}

async function ensureAgent(agentId, sessionId, userId, fields = {}) {
  await q(
    `INSERT INTO agents (id, session_id, user_id, name, type, subagent_type, status, parent_agent_id)
     VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
     ON CONFLICT (id) DO UPDATE SET
       status = COALESCE(EXCLUDED.status, agents.status),
       updated_at = now()`,
    [
      agentId,
      sessionId,
      userId,
      fields.name || "Main Agent",
      fields.type || "main",
      fields.subagent_type || null,
      fields.status || "working",
      fields.parent_agent_id || null,
    ]
  );
}

async function updateAgentSdd(agentId, { phaseKey, level, specDocId, specVersion }) {
  await q(
    `UPDATE agents SET
       sdd_phase = COALESCE($2, sdd_phase),
       sdd_level = COALESCE($3, sdd_level),
       spec_doc_id = COALESCE($4, spec_doc_id),
       spec_version = COALESCE($5, spec_version),
       updated_at = now()
     WHERE id = $1`,
    [
      agentId,
      phaseKey || null,
      level != null ? level : null,
      specDocId || null,
      specVersion || null,
    ]
  );
}

async function insertEvent({ sessionId, agentId, userId, eventType, toolName, summary, data }) {
  const r = await q(
    `INSERT INTO events (session_id, agent_id, user_id, event_type, tool_name, summary, data)
     VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, created_at`,
    [
      sessionId,
      agentId || null,
      userId,
      eventType,
      toolName || null,
      summary || null,
      data ? JSON.stringify(data) : null,
    ]
  );
  return r.rows[0];
}

// ── SDD ─────────────────────────────────────────────────────────────────────
async function insertSddActivity(a) {
  await q(
    `INSERT INTO sdd_activity
       (session_id, user_id, agent_id, hostname, hook_type, tool_name, phase, phase_index,
        phase_source, level, level_reason, lifecycle, spec_doc_id, spec_version,
        shared_core_touch, conflict_key, file_path, summary)
     VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
    [
      a.sessionId,
      a.userId,
      a.agentId || null,
      a.hostname || null,
      a.hookType || null,
      a.toolName || null,
      a.phase || null,
      a.phaseIndex != null ? a.phaseIndex : null,
      a.phaseSource || null,
      a.level != null ? a.level : null,
      a.levelReason || null,
      a.lifecycle || null,
      a.specDocId || null,
      a.specVersion || null,
      !!a.sharedCoreTouch,
      a.conflictKey || null,
      a.filePath || null,
      a.summary || null,
    ]
  );
}

async function upsertSpecVersion({ docId, version, title, source, publishedBy }) {
  await q(
    `INSERT INTO sdd_spec_versions (doc_id, version, title, source, published_by)
     VALUES ($1,$2,$3,$4,$5)
     ON CONFLICT (doc_id, version) DO UPDATE SET
       title = COALESCE(EXCLUDED.title, sdd_spec_versions.title)`,
    [docId, version, title || null, source || "hook", publishedBy || null]
  );
}

async function ensureGate(specDocId, phase) {
  await q(
    `INSERT INTO sdd_gates (spec_doc_id, phase) VALUES ($1,$2)
     ON CONFLICT (spec_doc_id, phase) DO NOTHING`,
    [specDocId, phase]
  );
}

async function setGate(specDocId, phase, status, approver, note) {
  await q(
    `INSERT INTO sdd_gates (spec_doc_id, phase, status, approver, note)
     VALUES ($1,$2,$3,$4,$5)
     ON CONFLICT (spec_doc_id, phase) DO UPDATE SET
       status = EXCLUDED.status, approver = EXCLUDED.approver,
       note = EXCLUDED.note, updated_at = now()`,
    [specDocId, phase, status, approver || null, note || null]
  );
  const r = await q("SELECT * FROM sdd_gates WHERE spec_doc_id=$1 AND phase=$2", [
    specDocId,
    phase,
  ]);
  return r.rows[0];
}

async function getGateForSpecPhase(specDocId, phase) {
  if (!specDocId || !phase) return null;
  const r = await q("SELECT status FROM sdd_gates WHERE spec_doc_id=$1 AND phase=$2", [
    specDocId,
    phase,
  ]);
  return r.rows[0] || null;
}

async function insertConflict({ kind, conflictKey, specDocId, detail }) {
  await q(
    `INSERT INTO sdd_conflicts (kind, conflict_key, spec_doc_id, detail)
     VALUES ($1,$2,$3,$4)`,
    [kind, conflictKey || null, specDocId || null, detail ? JSON.stringify(detail) : null]
  );
}

/** Distinct OTHER users who touched the same conflict_key within a window. */
async function otherUsersOnConflictKey(conflictKey, excludeUserId, windowMinutes) {
  const r = await q(
    `SELECT DISTINCT user_id FROM sdd_activity
     WHERE conflict_key = $1 AND user_id <> $2 AND shared_core_touch = true
       AND created_at >= now() - ($3 || ' minutes')::interval`,
    [conflictKey, excludeUserId, String(windowMinutes)]
  );
  return r.rows.map((x) => x.user_id);
}

// ── Tasks (coordination board) ──────────────────────────────────────────────
async function listTasks() {
  // Each task carries its assignee's name + their MOST RECENT live SDD signal
  // (phase/level/when) so the board shows real progress, not just a static card.
  const r = await q(`
    SELECT t.*, u.email AS assignee_email, u.name AS assignee_name,
           cu.name AS creator_name,
           la.phase AS live_phase, la.level AS live_level, la.created_at AS live_at
    FROM tasks t
    LEFT JOIN users u ON u.id = t.assignee_user_id
    LEFT JOIN users cu ON cu.id = t.created_by
    LEFT JOIN LATERAL (
      SELECT phase, level, created_at FROM sdd_activity sa
      WHERE sa.user_id = t.assignee_user_id
      ORDER BY sa.id DESC LIMIT 1
    ) la ON true
    ORDER BY t.priority DESC, t.updated_at DESC`);
  return r.rows;
}
async function createTask(t) {
  const r = await q(
    `INSERT INTO tasks (title, description, status, assignee_user_id, assignee_hostname, repo, spec_doc_id, priority, created_by)
     VALUES ($1,$2,COALESCE($3,'todo'),$4,$5,$6,$7,COALESCE($8,'normal'),$9) RETURNING *`,
    [
      t.title,
      t.description || null,
      t.status || null,
      t.assignee_user_id || null,
      t.assignee_hostname || null,
      t.repo || null,
      t.spec_doc_id || null,
      t.priority || null,
      t.created_by || null,
    ]
  );
  return r.rows[0];
}
async function updateTask(id, fields) {
  const sets = [],
    vals = [];
  for (const k of [
    "title",
    "description",
    "status",
    "assignee_user_id",
    "assignee_hostname",
    "repo",
    "spec_doc_id",
    "priority",
  ]) {
    if (fields[k] !== undefined) {
      vals.push(fields[k]);
      sets.push(`${k}=$${vals.length}`);
    }
  }
  if (!sets.length) {
    const r = await q("SELECT * FROM tasks WHERE id=$1", [id]);
    return r.rows[0];
  }
  vals.push(id);
  const r = await q(
    `UPDATE tasks SET ${sets.join(",")}, updated_at=now() WHERE id=$${vals.length} RETURNING *`,
    vals
  );
  return r.rows[0];
}
async function deleteTask(id) {
  await q("DELETE FROM tasks WHERE id=$1", [id]);
}
/** Candidate assignees: every known user + the hosts they report from. */
async function listAssignees() {
  const r = await q(`SELECT u.id, u.name, u.email,
    COALESCE(array_agg(DISTINCT h.hostname) FILTER (WHERE h.hostname IS NOT NULL), '{}') AS hosts
    FROM users u LEFT JOIN hosts h ON h.user_id=u.id GROUP BY u.id ORDER BY u.name NULLS LAST`);
  return r.rows;
}

module.exports = {
  pool,
  init,
  q,
  listTasks,
  createTask,
  updateTask,
  deleteTask,
  listAssignees,
  upsertUser,
  upsertHost,
  ensureSession,
  touchSession,
  setSessionStatus,
  ensureAgent,
  updateAgentSdd,
  insertEvent,
  insertSddActivity,
  upsertSpecVersion,
  ensureGate,
  setGate,
  getGateForSpecPhase,
  insertConflict,
  otherUsersOnConflictKey,
};
