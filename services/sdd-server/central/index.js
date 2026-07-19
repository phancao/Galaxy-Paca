/**
 * @file Galaxy SDD Coordination Server — central, online, multi-machine.
 *
 * Many developers' machines POST Claude Code hook events to /api/ingest
 * (authenticated by a Vortex delegation token). Each event is tagged with the
 * user + host, classified into SDD dimensions (reusing the per-machine monitor's
 * PURE classifier), persisted to Postgres, and fed to the cross-machine
 * coordination substrate (shared-core conflicts, shared gates, spec drift). A
 * browser team dashboard (the same React UI) reads the aggregated team API.
 */

const path = require("path");
const http = require("http");
const express = require("express");
const cors = require("cors");
const { WebSocketServer } = require("ws");

const db = require("./db");
const { requireAuth, requireRead } = require("./auth");
const { startPacaBridge } = require("./paca-bridge");

// Paca signal bridge (ADR-038 T6). Inert no-op until start() replaces it; the
// real bridge is fire-and-forget and must never fail or slow ingest.
let pacaBridge = { nudge() {} };

// OIDC config the SPA needs to start a login (public — no secrets). When
// SDD_OIDC=on the browser must log into Vortex before the read API answers.
const OIDC_ENABLED = (process.env.SDD_OIDC || "on") !== "off";
const OIDC_CLIENT_ID = process.env.SDD_OIDC_CLIENT_ID || "sdd-server";
// Reuse the monitor's PURE SDD classifier + rules (no sqlite dependency).
const { classify } = require("../server/lib/sdd-classify");
const { loadRules } = require("../server/lib/sdd-rules");

const PORT = parseInt(process.env.PORT || "4830", 10);
const SHARED_CORE_WINDOW_MIN = parseInt(process.env.SDD_CONFLICT_WINDOW_MIN || "30", 10);

const app = express();
app.use(cors());
app.use(express.json({ limit: "2mb" }));

// Public: the SPA fetches this to decide whether to force a Vortex login.
app.get("/api/auth-config", (req, res) =>
  res.json({ oidc: OIDC_ENABLED, client_id: OIDC_CLIENT_ID })
);

// Gate the read API behind a logged-in Vortex identity (when OIDC is on).
// /api/ingest keeps its own delegation-token auth; health + auth-config stay open.
app.use((req, res, next) => {
  if (!OIDC_ENABLED || !req.path.startsWith("/api/")) return next();
  if (["/api/health", "/api/auth-config", "/api/ingest"].includes(req.path)) return next();
  return requireRead(req, res, next);
});

const server = http.createServer(app);
const wss = new WebSocketServer({ server, path: "/ws" });
function broadcast(type, data) {
  const msg = JSON.stringify({ type, data, timestamp: new Date().toISOString() });
  for (const c of wss.clients)
    if (c.readyState === 1)
      try {
        c.send(msg);
      } catch {
        /* ignore */
      }
}

// ── Helpers ─────────────────────────────────────────────────────────────────
function dirname(p) {
  if (!p) return null;
  const norm = String(p).replace(/\\/g, "/");
  const i = norm.lastIndexOf("/");
  return i > 0 ? norm.slice(0, i) : norm;
}
async function getAgentPhase(agentId) {
  if (!agentId) return null;
  const r = await db.q("SELECT sdd_phase FROM agents WHERE id=$1", [agentId]);
  return r.rows[0] ? r.rows[0].sdd_phase : null;
}

// ── Ingest ──────────────────────────────────────────────────────────────────
app.post("/api/ingest", requireAuth, async (req, res) => {
  const { hook_type, data } = req.body || {};
  if (!hook_type || !data || !data.session_id) {
    return res
      .status(400)
      .json({ error: { code: "INVALID_INPUT", message: "hook_type + data.session_id required" } });
  }
  const userId = req.actor.sub;
  const hostname = req.headers["x-sdd-host"] || data.host || data.hostname || null;

  // Respond fast; the heavy lifting is fire-and-forget (hooks must never block).
  res.json({ ok: true });

  try {
    await db.upsertUser(req.actor);
    await db.upsertHost(userId, hostname);
    await db.ensureSession(data.session_id, userId, hostname, data);

    // One main agent per session; subagents created on PreToolUse Agent.
    const mainId = `${data.session_id}-main`;
    await db.ensureAgent(mainId, data.session_id, userId, { name: "Main Agent", type: "main" });
    let agentId = mainId;
    if (hook_type === "PreToolUse" && data.tool_name === "Agent") {
      const input = data.tool_input || {};
      const subId = `${data.session_id}-sub-${Date.now()}`;
      await db.ensureAgent(subId, data.session_id, userId, {
        name: (input.description || input.subagent_type || "Subagent").slice(0, 60),
        type: "subagent",
        subagent_type: input.subagent_type || null,
        parent_agent_id: mainId,
      });
      agentId = subId;
    }

    if (hook_type === "SessionEnd") await db.setSessionStatus(data.session_id, "completed", true);
    else await db.touchSession(data.session_id);

    const summary = hook_type === "PreToolUse" ? `Using tool: ${data.tool_name}` : hook_type;
    const ev = await db.insertEvent({
      sessionId: data.session_id,
      agentId,
      userId,
      eventType: hook_type,
      toolName: data.tool_name || null,
      summary,
      data,
    });
    broadcast("new_event", {
      session_id: data.session_id,
      user_id: userId,
      event_type: hook_type,
      created_at: ev.created_at,
    });

    // ── SDD classification ──
    const rules = loadRules();
    const agentPhase = await getAgentPhase(agentId);
    const c = classify({
      hookType: hook_type,
      eventType: hook_type,
      toolName: data.tool_name,
      data,
      agentPhase,
      rules,
    });

    const meaningful =
      c.phaseSource === "command" || (c.level != null && c.level >= 2) || !!c.lifecycle;
    if (!meaningful) return;

    // Cross-user conflict key: repo + shared-core directory touched.
    const repoRow = await db.q("SELECT repo FROM sessions WHERE id=$1", [data.session_id]);
    const repo = (repoRow.rows[0] && repoRow.rows[0].repo) || "norepo";
    const conflictKey = c.sharedCoreTouch ? `${repo}:${dirname(c.filePath)}` : null;

    await db.insertSddActivity({
      sessionId: data.session_id,
      userId,
      agentId,
      hostname,
      hookType: hook_type,
      toolName: data.tool_name,
      phase: c.phaseKey,
      phaseIndex: c.phase,
      phaseSource: c.phaseSource,
      level: c.level,
      levelReason: c.levelReason,
      lifecycle: c.lifecycle,
      specDocId: c.spec ? c.spec.docId : null,
      specVersion: c.spec ? c.spec.version : null,
      sharedCoreTouch: c.sharedCoreTouch,
      conflictKey,
      filePath: c.filePath,
      summary,
    });
    if (c.phaseKey || c.level != null || c.spec) {
      await db.updateAgentSdd(agentId, {
        phaseKey: c.phaseKey,
        level: c.level,
        specDocId: c.spec ? c.spec.docId : null,
        specVersion: c.spec ? c.spec.version : null,
      });
    }
    if (c.lifecycle === "publish" && c.spec && c.spec.version) {
      await db.upsertSpecVersion({
        docId: c.spec.docId || "unknown",
        version: c.spec.version,
        title: (data.tool_input && (data.tool_input.title || data.tool_input.label)) || null,
        source: "hook",
        publishedBy: userId,
      });
      pacaBridge.nudge(); // Signal 2: spec version → Paca (fire-and-forget)
    }
    if (c.phaseKey && c.spec && c.spec.docId) await db.ensureGate(c.spec.docId, c.phaseKey);

    broadcast("sdd_updated", {
      session_id: data.session_id,
      user_id: userId,
      phase: c.phaseKey,
      level: c.level,
      lifecycle: c.lifecycle,
      shared_core_touch: c.sharedCoreTouch,
    });

    // ── Coordination: cross-machine Shared Core conflict ──
    if (c.sharedCoreTouch && conflictKey) {
      const others = await db.otherUsersOnConflictKey(conflictKey, userId, SHARED_CORE_WINDOW_MIN);
      if (others.length) {
        await db.insertConflict({
          kind: "shared_core_parallel",
          conflictKey,
          specDocId: c.spec ? c.spec.docId : null,
          detail: {
            users: [userId, ...others],
            where: conflictKey,
            window_min: SHARED_CORE_WINDOW_MIN,
          },
        });
        broadcast("sdd_conflict", {
          kind: "shared_core_parallel",
          conflict_key: conflictKey,
          users: [userId, ...others],
        });
        pacaBridge.nudge(); // Signal 1: conflict → Paca (fire-and-forget)
      }
    }
  } catch (err) {
    console.error("[INGEST] failed:", err.message);
  }
});

// ── Team API ────────────────────────────────────────────────────────────────
app.get("/api/health", (req, res) =>
  res.json({ status: "ok", timestamp: new Date().toISOString() })
);

app.get("/api/stats", async (req, res) => {
  try {
    const r = await db.q(`SELECT
      (SELECT COUNT(*) FROM users)::int AS total_users,
      (SELECT COUNT(*) FROM sessions)::int AS total_sessions,
      (SELECT COUNT(*) FROM sessions WHERE status='active')::int AS active_sessions,
      (SELECT COUNT(*) FROM agents)::int AS total_agents,
      (SELECT COUNT(*) FROM events)::int AS total_events`);
    res.json(r.rows[0]);
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

app.get("/api/sessions", async (req, res) => {
  try {
    const r = await db.q(`SELECT s.*, u.email, u.name AS user_name FROM sessions s
      JOIN users u ON u.id=s.user_id ORDER BY s.updated_at DESC LIMIT 200`);
    res.json({ sessions: r.rows, total: r.rowCount });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

app.get("/api/sdd", async (req, res) => {
  try {
    const phases = loadRules().phases;
    const pc = await db.q(
      "SELECT phase, COUNT(*)::int n FROM sdd_activity WHERE phase IS NOT NULL GROUP BY phase"
    );
    const lc = await db.q(
      "SELECT level, COUNT(*)::int n FROM sdd_activity WHERE level IS NOT NULL GROUP BY level"
    );
    const agents =
      await db.q(`SELECT a.id,a.name,a.type,a.status,a.session_id,a.sdd_phase,a.sdd_level,a.spec_doc_id,a.spec_version,
      u.email,u.name AS user_name FROM agents a JOIN users u ON u.id=a.user_id WHERE a.sdd_phase IS NOT NULL ORDER BY a.updated_at DESC LIMIT 300`);
    const recent = await db.q(
      `SELECT sa.*, u.email FROM sdd_activity sa JOIN users u ON u.id=sa.user_id ORDER BY sa.id DESC LIMIT 50`
    );
    const sc = await db.q("SELECT COUNT(*)::int n FROM sdd_activity WHERE shared_core_touch=true");
    const ul3 = await db.q(`SELECT COUNT(*)::int n FROM sdd_activity sa
      LEFT JOIN sdd_gates g ON g.spec_doc_id=sa.spec_doc_id AND g.phase=sa.phase
      WHERE sa.level>=3 AND (g.status IS NULL OR g.status<>'approved')`);
    const phaseCounts = {};
    for (const x of pc.rows) phaseCounts[x.phase] = x.n;
    const levelCounts = { 1: 0, 2: 0, 3: 0, 4: 0 };
    for (const x of lc.rows) levelCounts[x.level] = x.n;
    const board = {};
    for (const p of phases) board[p.key] = [];
    for (const a of agents.rows) if (board[a.sdd_phase]) board[a.sdd_phase].push(a);
    res.json({
      phases,
      phaseCounts,
      levelCounts,
      board,
      recent: recent.rows,
      sharedCoreCount: sc.rows[0].n,
      unapprovedL3Count: ul3.rows[0].n,
    });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

app.get("/api/sdd/spec-versions", async (req, res) => {
  try {
    const r = await db.q("SELECT * FROM sdd_spec_versions ORDER BY doc_id, published_at");
    const docs = {};
    for (const v of r.rows) (docs[v.doc_id] = docs[v.doc_id] || []).push(v);
    res.json({ docs, count: r.rowCount });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

app.get("/api/sdd/flags", async (req, res) => {
  try {
    const shared = await db.q(
      "SELECT * FROM sdd_activity WHERE shared_core_touch=true ORDER BY id DESC LIMIT 200"
    );
    const ul3 = await db.q(`SELECT sa.* FROM sdd_activity sa
      LEFT JOIN sdd_gates g ON g.spec_doc_id=sa.spec_doc_id AND g.phase=sa.phase
      WHERE sa.level>=3 AND (g.status IS NULL OR g.status<>'approved') ORDER BY sa.id DESC LIMIT 200`);
    res.json({ sharedCore: shared.rows, unapprovedL3: ul3.rows });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

app.get("/api/sdd/spec-sync/status", (req, res) =>
  res.json({ configured: false, hasToken: false })
);

app.get("/api/sdd/conflicts", async (req, res) => {
  try {
    const r = await db.q(
      "SELECT * FROM sdd_conflicts WHERE status='open' ORDER BY id DESC LIMIT 100"
    );
    res.json({ conflicts: r.rows });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

// Team board: who is working on what (active sessions by user x repo).
app.get("/api/sdd/team", async (req, res) => {
  try {
    const r = await db.q(`SELECT u.id user_id, u.email, u.name, s.repo, s.hostname, s.id session_id,
      s.status, s.updated_at, a.sdd_phase, a.sdd_level, a.spec_doc_id, a.spec_version
      FROM sessions s JOIN users u ON u.id=s.user_id
      LEFT JOIN agents a ON a.id = s.id||'-main'
      WHERE s.status='active' ORDER BY s.updated_at DESC LIMIT 200`);
    res.json({ members: r.rows });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

app.post("/api/sdd/gates", async (req, res) => {
  try {
    const { spec_doc_id, phase, status, approver, note } = req.body || {};
    if (!spec_doc_id || !phase || !status)
      return res.status(400).json({ error: { message: "spec_doc_id, phase, status required" } });
    const gate = await db.setGate(spec_doc_id, phase, status, approver, note);
    broadcast("sdd_updated", { gate });
    res.json({ ok: true, gate });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

// ── Read endpoints the reused UI's Sessions / Activity tabs need ────────────
app.get("/api/sessions/facets", async (req, res) => {
  try {
    const r = await db.q(
      "SELECT DISTINCT cwd FROM sessions WHERE cwd IS NOT NULL ORDER BY cwd LIMIT 200"
    );
    res.json({ cwds: r.rows.map((x) => x.cwd) });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});
app.get("/api/stats/facets", async (req, res) => {
  try {
    const r = await db.q(
      "SELECT DISTINCT cwd FROM sessions WHERE cwd IS NOT NULL ORDER BY cwd LIMIT 200"
    );
    res.json({ cwds: r.rows.map((x) => x.cwd) });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

app.get("/api/agents", async (req, res) => {
  try {
    const status = req.query.status;
    const sql = status
      ? `SELECT a.*, u.email FROM agents a JOIN users u ON u.id=a.user_id WHERE a.status=$1 ORDER BY a.updated_at DESC LIMIT 500`
      : `SELECT a.*, u.email FROM agents a JOIN users u ON u.id=a.user_id ORDER BY a.updated_at DESC LIMIT 500`;
    const r = await db.q(sql, status ? [status] : []);
    res.json({ agents: r.rows });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

app.get("/api/events", async (req, res) => {
  try {
    const limit = Math.min(parseInt(req.query.limit, 10) || 50, 200);
    const offset = parseInt(req.query.offset, 10) || 0;
    const sid = req.query.session_id;
    const where = sid ? "WHERE session_id=$3" : "";
    const params = sid ? [limit, offset, sid] : [limit, offset];
    const r = await db.q(
      `SELECT e.*, u.email FROM events e JOIN users u ON u.id=e.user_id ${where} ORDER BY e.id DESC LIMIT $1 OFFSET $2`,
      params
    );
    const tot = await db.q(
      `SELECT COUNT(*)::int n FROM events ${sid ? "WHERE session_id=$1" : ""}`,
      sid ? [sid] : []
    );
    res.json({ events: r.rows, total: tot.rows[0].n, limit, offset });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});
app.get("/api/events/facets", async (req, res) => {
  try {
    const et = await db.q("SELECT DISTINCT event_type FROM events ORDER BY event_type LIMIT 100");
    const tn = await db.q(
      "SELECT DISTINCT tool_name FROM events WHERE tool_name IS NOT NULL ORDER BY tool_name LIMIT 200"
    );
    res.json({
      event_types: et.rows.map((x) => x.event_type),
      tool_names: tn.rows.map((x) => x.tool_name),
    });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

app.get("/api/sessions/:id", async (req, res) => {
  try {
    const s = await db.q(
      "SELECT s.*, u.email FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.id=$1",
      [req.params.id]
    );
    if (!s.rows[0]) return res.status(404).json({ error: { message: "not found" } });
    const agents = await db.q("SELECT * FROM agents WHERE session_id=$1 ORDER BY started_at", [
      req.params.id,
    ]);
    const events = await db.q(
      "SELECT * FROM events WHERE session_id=$1 ORDER BY id DESC LIMIT 500",
      [req.params.id]
    );
    res.json({ session: s.rows[0], agents: agents.rows, events: events.rows, workflows: [] });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

// ── Tasks: cross-machine coordination board ─────────────────────────────────
app.get("/api/tasks", async (req, res) => {
  try {
    res.json({
      tasks: await db.listTasks(),
      statuses: ["todo", "assigned", "in_progress", "review", "done"],
    });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});
app.get("/api/tasks/assignees", async (req, res) => {
  try {
    res.json({ assignees: await db.listAssignees() });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});
app.post("/api/tasks", async (req, res) => {
  try {
    const b = req.body || {};
    if (!b.title) return res.status(400).json({ error: { message: "title required" } });
    const t = await db.createTask({ ...b, created_by: req.actor.sub });
    broadcast("task_updated", t);
    res.json({ ok: true, task: t });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});
app.patch("/api/tasks/:id", async (req, res) => {
  try {
    const t = await db.updateTask(req.params.id, req.body || {});
    if (!t) return res.status(404).json({ error: { message: "not found" } });
    broadcast("task_updated", t);
    res.json({ ok: true, task: t });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});
app.delete("/api/tasks/:id", async (req, res) => {
  try {
    await db.deleteTask(req.params.id);
    broadcast("task_updated", { id: req.params.id, deleted: true });
    res.json({ ok: true });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

// ── Team coordination aggregates (Dashboard / Analytics / Fleet / Workflows) ─
app.get("/api/team/overview", async (req, res) => {
  try {
    const o = await db.q(`SELECT
      (SELECT COUNT(DISTINCT hostname)::int FROM sessions WHERE updated_at > now()-interval '15 min') AS machines_online,
      (SELECT COUNT(*)::int FROM hosts) AS machines_total,
      (SELECT COUNT(DISTINCT user_id)::int FROM sessions WHERE status='active') AS active_devs,
      (SELECT COUNT(*)::int FROM users) AS total_users,
      (SELECT COUNT(*)::int FROM sessions) AS total_sessions,
      (SELECT COUNT(*)::int FROM sessions WHERE status='active') AS active_sessions,
      (SELECT COUNT(*)::int FROM events) AS total_events,
      (SELECT COUNT(*)::int FROM sdd_conflicts WHERE status='open') AS open_conflicts,
      (SELECT COUNT(*)::int FROM sdd_gates WHERE status='pending') AS pending_gates`);
    const tasks = await db.q("SELECT status, COUNT(*)::int n FROM tasks GROUP BY status");
    const tasksByStatus = {};
    for (const r of tasks.rows) tasksByStatus[r.status] = r.n;
    const recent =
      await db.q(`SELECT sa.phase, sa.level, sa.tool_name, sa.created_at, sa.hostname, u.name AS user_name
      FROM sdd_activity sa JOIN users u ON u.id=sa.user_id ORDER BY sa.id DESC LIMIT 15`);
    res.json({ ...o.rows[0], tasksByStatus, recent: recent.rows });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

app.get("/api/team/analytics", async (req, res) => {
  try {
    const byUser = await db.q(`SELECT COALESCE(u.name,u.email,u.id) AS label, COUNT(*)::int n
      FROM sdd_activity sa JOIN users u ON u.id=sa.user_id GROUP BY 1 ORDER BY n DESC LIMIT 20`);
    const byHost = await db.q(
      `SELECT COALESCE(hostname,'?') AS label, COUNT(*)::int n FROM sdd_activity GROUP BY 1 ORDER BY n DESC LIMIT 20`
    );
    const byRepo = await db.q(`SELECT COALESCE(s.repo,'?') AS label, COUNT(*)::int n
      FROM sdd_activity sa JOIN sessions s ON s.id=sa.session_id GROUP BY 1 ORDER BY n DESC LIMIT 20`);
    const phaseDist = await db.q(
      `SELECT phase AS label, COUNT(*)::int n FROM sdd_activity WHERE phase IS NOT NULL GROUP BY 1 ORDER BY n DESC`
    );
    const levelDist = await db.q(
      `SELECT level, COUNT(*)::int n FROM sdd_activity WHERE level IS NOT NULL GROUP BY 1 ORDER BY level`
    );
    const daily =
      await db.q(`SELECT to_char(date_trunc('day',created_at),'MM-DD') AS day, COUNT(*)::int n
      FROM events WHERE created_at > now()-interval '14 days' GROUP BY 1 ORDER BY 1`);
    res.json({
      byUser: byUser.rows,
      byHost: byHost.rows,
      byRepo: byRepo.rows,
      phaseDist: phaseDist.rows,
      levelDist: levelDist.rows,
      daily: daily.rows,
    });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

app.get("/api/team/fleet", async (req, res) => {
  try {
    const r =
      await db.q(`SELECT u.id AS user_id, COALESCE(u.name,u.email,u.id) AS user_name, u.email,
        s.hostname,
        COUNT(DISTINCT s.id)::int AS sessions,
        MAX(s.updated_at) AS last_seen,
        (SELECT a.sdd_phase FROM agents a WHERE a.user_id=u.id AND a.sdd_phase IS NOT NULL ORDER BY a.updated_at DESC LIMIT 1) AS current_phase,
        (SELECT a.sdd_level FROM agents a WHERE a.user_id=u.id AND a.sdd_level IS NOT NULL ORDER BY a.updated_at DESC LIMIT 1) AS current_level
      FROM users u LEFT JOIN sessions s ON s.user_id=u.id
      GROUP BY u.id, u.name, u.email, s.hostname ORDER BY last_seen DESC NULLS LAST LIMIT 200`);
    res.json({ machines: r.rows });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

app.get("/api/team/coordination", async (req, res) => {
  try {
    const conflicts = await db.q(
      "SELECT * FROM sdd_conflicts WHERE status='open' ORDER BY id DESC LIMIT 100"
    );
    const byRepo = await db.q(`SELECT COALESCE(s.repo,'?') AS repo,
        COUNT(DISTINCT s.user_id)::int AS devs,
        COUNT(DISTINCT s.id)::int AS sessions,
        COUNT(DISTINCT s.hostname)::int AS machines,
        array_remove(array_agg(DISTINCT a.sdd_phase), NULL) AS phases,
        array_remove(array_agg(DISTINCT u.name), NULL) AS dev_names
      FROM sessions s JOIN users u ON u.id=s.user_id
      LEFT JOIN agents a ON a.session_id=s.id
      WHERE s.repo IS NOT NULL GROUP BY s.repo ORDER BY sessions DESC LIMIT 50`);
    res.json({ conflicts: conflicts.rows, byRepo: byRepo.rows });
  } catch (e) {
    res.status(500).json({ error: { message: e.message } });
  }
});

// Benign fallbacks for the monitor-only surfaces the coordination UI hides but
// might still be probed (keeps the app from 404-crashing if a stray call slips).
app.get(
  /^\/api\/(pricing|analytics|workflows|settings|updates|alerts|webhooks|cc-config|run|push).*/,
  (req, res) =>
    res.json({
      items: [],
      agents: [],
      events: [],
      sessions: [],
      runs: [],
      rules: [],
      targets: [],
      providers: [],
      pricing: [],
      total: 0,
      total_cost: 0,
      breakdown: [],
      daily_costs: [],
    })
);

// ── Static UI (the same React dashboard) ────────────────────────────────────
// Decommission switch (ADR-038): the SDD fleet dashboard is now NATIVE inside
// Paca (plugin com.galaxy.sdd, reached through the same-origin /sdd-api proxy),
// so the standalone SPA is retired. With SDD_SERVE_SPA=off the server keeps
// /api/* (consumed internally over galaxy_network by the Paca sdd-proxy) and
// /ws + /api/ingest (agents), but stops serving the browser dashboard and
// points humans to the new home. Defaults to serving the SPA (dev / rollback).
const SERVE_SPA = (process.env.SDD_SERVE_SPA || "on") !== "off";
const MOVED_HTML =
  '<!doctype html><html lang="en"><head><meta charset="utf-8">' +
  '<meta name="viewport" content="width=device-width,initial-scale=1">' +
  "<title>SDD Fleet moved</title><style>" +
  "body{font:15px/1.6 system-ui,-apple-system,Segoe UI,sans-serif;background:#14181f;color:#e8eaed;" +
  "display:flex;min-height:100vh;margin:0;align-items:center;justify-content:center}" +
  ".c{max-width:520px;padding:32px;text-align:center}a{color:#34d39e}</style></head><body><div class=c>" +
  "<h1>SDD Fleet has moved</h1><p>The Spec-Driven Development fleet dashboard is now built into " +
  "Galaxy Tasks (Paca) as a native view — open a project and pick <strong>SDD Fleet</strong> " +
  'in the sidebar.</p><p><a href="https://tasks.skyplatform.net">Go to Galaxy Tasks →</a></p>' +
  "<p style=opacity:.6;font-size:12px>The sensor keeps collecting telemetry; only the standalone " +
  "dashboard is retired (ADR-038).</p></div></body></html>";

if (SERVE_SPA) {
  const CLIENT_DIST = path.join(__dirname, "..", "client", "dist");
  app.use(express.static(CLIENT_DIST));
  app.get(/^(?!\/api|\/ws).*/, (req, res) => res.sendFile(path.join(CLIENT_DIST, "index.html")));
} else {
  app.get(/^(?!\/api|\/ws).*/, (req, res) => res.status(410).type("html").send(MOVED_HTML));
}

async function start() {
  await db.init();
  pacaBridge = startPacaBridge({ db, log: console }); // inert unless PACA_BRIDGE_ENABLED
  server.listen(PORT, () => console.log(`Galaxy SDD Coordination Server on :${PORT}`));
}
if (require.main === module)
  start().catch((e) => {
    console.error("Fatal:", e);
    process.exit(1);
  });

module.exports = { app, start, broadcast };
