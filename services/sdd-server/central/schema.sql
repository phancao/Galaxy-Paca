-- Galaxy SDD Coordination Server — central Postgres schema.
--
-- This backs the ONLINE, multi-machine coordination server: many developers on
-- different machines / Claude accounts ship hook events here (authenticated by a
-- Vortex delegation token), the server classifies them into SDD dimensions and
-- coordinates across the whole team. It deliberately stores ONLY what arrives
-- over the wire from hooks — no local-only artifacts (transcripts, cost-from-
-- JSONL, workflow journals) which can't exist server-side.
--
-- Multi-tenancy: every session/agent/event is tagged with the user (resolved
-- from the delegation token's `sub`/`email`) and the originating host machine.

-- ── Identity ────────────────────────────────────────────────────────────────
-- Mirror of the Vortex user, upserted from delegation-token claims on ingest.
CREATE TABLE IF NOT EXISTS users (
  id          TEXT PRIMARY KEY,            -- Vortex `sub` (UUID string)
  email       TEXT,
  name        TEXT,
  first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Distinct dev machines a user reports from (a user may run several).
CREATE TABLE IF NOT EXISTS hosts (
  id          BIGINT GENERATED ALWAYS AS IDENTITY,
  user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  hostname    TEXT NOT NULL,
  first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (user_id, hostname)
);

-- ── Core event stream (tagged by user + host) ───────────────────────────────
CREATE TABLE IF NOT EXISTS sessions (
  id          TEXT PRIMARY KEY,            -- Claude Code session id (per machine)
  user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  hostname    TEXT,
  status      TEXT NOT NULL DEFAULT 'active'
                CHECK (status IN ('active','completed','error','abandoned')),
  cwd         TEXT,
  repo        TEXT,                         -- git remote / toplevel, for grouping
  model       TEXT,
  started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  ended_at    TIMESTAMPTZ,
  metadata    JSONB
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
CREATE INDEX IF NOT EXISTS idx_sessions_repo ON sessions(repo);

CREATE TABLE IF NOT EXISTS agents (
  id              TEXT PRIMARY KEY,
  session_id      TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name            TEXT NOT NULL,
  type            TEXT NOT NULL DEFAULT 'main' CHECK (type IN ('main','subagent')),
  subagent_type   TEXT,
  status          TEXT NOT NULL DEFAULT 'waiting'
                    CHECK (status IN ('working','waiting','completed','error')),
  parent_agent_id TEXT,
  -- SDD state (current), mirrors the per-machine monitor's agent columns.
  sdd_phase       TEXT,
  sdd_level       INTEGER,
  spec_doc_id     TEXT,
  spec_version    TEXT,
  started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  ended_at        TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_agents_session ON agents(session_id);
CREATE INDEX IF NOT EXISTS idx_agents_user ON agents(user_id);

CREATE TABLE IF NOT EXISTS events (
  id          BIGINT GENERATED ALWAYS AS IDENTITY,
  session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  agent_id    TEXT,
  user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  event_type  TEXT NOT NULL,
  tool_name   TEXT,
  summary     TEXT,
  data        JSONB,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_events_session ON events(session_id);
CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at DESC);

-- ── SDD classification (per-event, the coordination substrate) ──────────────
CREATE TABLE IF NOT EXISTS sdd_activity (
  id                BIGINT GENERATED ALWAYS AS IDENTITY,
  session_id        TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  user_id           TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  agent_id          TEXT,
  hostname          TEXT,
  hook_type         TEXT,
  tool_name         TEXT,
  phase             TEXT,
  phase_index       INTEGER,
  phase_source      TEXT,
  level             INTEGER,
  level_reason      TEXT,
  lifecycle         TEXT,
  spec_doc_id       TEXT,
  spec_version      TEXT,
  shared_core_touch BOOLEAN NOT NULL DEFAULT false,
  -- Normalized "what was touched" key for cross-user conflict detection
  -- (e.g. repo + shared-core module, or a contract/schema path).
  conflict_key      TEXT,
  file_path         TEXT,
  summary           TEXT,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_sdd_activity_user ON sdd_activity(user_id);
CREATE INDEX IF NOT EXISTS idx_sdd_activity_phase ON sdd_activity(phase);
CREATE INDEX IF NOT EXISTS idx_sdd_activity_level ON sdd_activity(level);
CREATE INDEX IF NOT EXISTS idx_sdd_activity_conflict ON sdd_activity(conflict_key, created_at DESC);

-- Spec versions tracked team-wide (published / pinned across machines).
CREATE TABLE IF NOT EXISTS sdd_spec_versions (
  id              BIGINT GENERATED ALWAYS AS IDENTITY,
  doc_id          TEXT NOT NULL,
  version         TEXT NOT NULL,
  title           TEXT,
  source          TEXT NOT NULL DEFAULT 'hook',
  published_by    TEXT REFERENCES users(id) ON DELETE SET NULL,
  implemented_ref TEXT,
  published_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  bridged_at      TIMESTAMPTZ,              -- when the Paca bridge emitted this signal (ADR-038 T6)
  UNIQUE (doc_id, version)
);
CREATE INDEX IF NOT EXISTS idx_sdd_spec_versions_doc ON sdd_spec_versions(doc_id);

-- ── Coordination: SHARED gates keyed by (spec_doc_id, phase) ────────────────
-- A lead approves a phase gate for a spec ONCE; every machine/agent working on
-- that spec sees it. This is the team-wide version of the per-session gate.
CREATE TABLE IF NOT EXISTS sdd_gates (
  id          BIGINT GENERATED ALWAYS AS IDENTITY,
  spec_doc_id TEXT NOT NULL,
  phase       TEXT NOT NULL,
  status      TEXT NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending','approved','rejected')),
  approver    TEXT REFERENCES users(id) ON DELETE SET NULL,
  note        TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (spec_doc_id, phase)
);

-- ── Coordination: cross-user conflicts + drift, surfaced to the team ────────
CREATE TABLE IF NOT EXISTS sdd_conflicts (
  id            BIGINT GENERATED ALWAYS AS IDENTITY,
  kind          TEXT NOT NULL CHECK (kind IN ('shared_core_parallel','l3_unapproved','spec_drift')),
  conflict_key  TEXT,                       -- shared module / spec touched
  spec_doc_id   TEXT,
  detail        JSONB,                      -- {users:[...], versions:[...], ...}
  status        TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','acknowledged','resolved')),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  bridged_at    TIMESTAMPTZ                 -- when the Paca bridge emitted this signal (ADR-038 T6)
);
CREATE INDEX IF NOT EXISTS idx_sdd_conflicts_status ON sdd_conflicts(status, created_at DESC);

-- Paca-bridge bookkeeping (ADR-038 T6): NULL = signal not yet emitted into
-- Paca. schema.sql is re-applied on every boot (db.init), so databases created
-- before this column pick it up via these guarded ALTERs — the CREATE TABLE
-- IF NOT EXISTS above is a no-op for them.
ALTER TABLE sdd_conflicts     ADD COLUMN IF NOT EXISTS bridged_at TIMESTAMPTZ;
ALTER TABLE sdd_spec_versions ADD COLUMN IF NOT EXISTS bridged_at TIMESTAMPTZ;

-- ── Coordination: TASK board (cross-machine work dispatch + tracking) ───────
-- A lead creates a task and assigns it to a developer (and optionally a
-- specific machine). The assignee's machine "receives" it; the board tracks
-- progress by joining the assignee's live SDD activity. This is the team-wide
-- replacement for the per-machine Kanban.
CREATE TABLE IF NOT EXISTS tasks (
  id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  title             TEXT NOT NULL,
  description       TEXT,
  status            TEXT NOT NULL DEFAULT 'todo'
                      CHECK (status IN ('todo','assigned','in_progress','review','done')),
  assignee_user_id  TEXT REFERENCES users(id) ON DELETE SET NULL,
  assignee_hostname TEXT,
  repo              TEXT,
  spec_doc_id       TEXT,
  priority          TEXT NOT NULL DEFAULT 'normal' CHECK (priority IN ('low','normal','high')),
  created_by        TEXT REFERENCES users(id) ON DELETE SET NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_assignee ON tasks(assignee_user_id);

-- ADR-036 tenant isolation Stage A (T1+T4). Every public base table gets tenant_id
-- + GUC-default trigger + FORCE RLS (permissive-when-unset). Idempotent.
-- Stage B (out-of-band): non-super sdd_app owns tables; repoint PGUSER sdd->sdd_app.
-- Stage C (SET LOCAL per request) TODO: sdd-server is Node (pg pool) — needs
-- pool.connect()+set_config per request to activate isolation.
CREATE OR REPLACE FUNCTION sdd_tenant_from_guc() RETURNS trigger AS $fn$
BEGIN
  IF NEW.tenant_id IS NULL THEN NEW.tenant_id := nullif(current_setting('app.current_tenant', true), '')::uuid; END IF;
  RETURN NEW;
END $fn$ LANGUAGE plpgsql;
DO $do$
DECLARE r record;
  pred text := '(current_setting(''app.current_tenant'', true) IS NULL OR current_setting(''app.current_tenant'', true) = '''' OR tenant_id IS NULL OR tenant_id::text = current_setting(''app.current_tenant'', true))';
BEGIN
  FOR r IN SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename NOT LIKE '\_\_%'
  LOOP
    EXECUTE format('ALTER TABLE public.%I ADD COLUMN IF NOT EXISTS tenant_id uuid', r.tablename);
    EXECUTE format('DROP TRIGGER IF EXISTS trg_tenant ON public.%I', r.tablename);
    EXECUTE format('CREATE TRIGGER trg_tenant BEFORE INSERT OR UPDATE ON public.%I FOR EACH ROW EXECUTE FUNCTION sdd_tenant_from_guc()', r.tablename);
    IF NOT EXISTS (SELECT 1 FROM pg_policies p WHERE p.schemaname='public' AND p.tablename=r.tablename) THEN
      EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY', r.tablename);
      EXECUTE format('ALTER TABLE public.%I FORCE ROW LEVEL SECURITY', r.tablename);
      EXECUTE format('CREATE POLICY tenant_isolation ON public.%I USING %s WITH CHECK %s', r.tablename, pred, pred);
    END IF;
  END LOOP;
END $do$;
