-- 000027_add_versions_components_worklogs.sql
-- ADR-040 (advanced task schema, Phase 2): three new, self-contained aggregates.
--
--   versions       project release / fix-version (Jira-style fixVersion). A task
--                  can be targeted at a release; released/archived are lifecycle
--                  flags, release_date the planned or actual ship date.
--   components      a functional area of a project, optionally owned by a lead
--                  project member (lead_member_id → project_members).
--   task_worklogs   time-tracking entries logged against a task, attributed to
--                   the acting project member (nullable so a non-member actor
--                   can still log work).
--
-- All three tables are independent of the tasks table's own columns; the
-- task-side change that consumes versions/components owns those fields.

BEGIN;

CREATE TABLE IF NOT EXISTS versions (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    description  TEXT,
    released     BOOLEAN     NOT NULL DEFAULT false,
    release_date DATE,
    archived     BOOLEAN     NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, name)
);

CREATE INDEX IF NOT EXISTS idx_versions_project ON versions(project_id);

CREATE TABLE IF NOT EXISTS components (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id     UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name           TEXT        NOT NULL,
    description    TEXT,
    lead_member_id UUID        REFERENCES project_members(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, name)
);

CREATE INDEX IF NOT EXISTS idx_components_project ON components(project_id);

CREATE TABLE IF NOT EXISTS task_worklogs (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id    UUID        NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    member_id  UUID        REFERENCES project_members(id) ON DELETE SET NULL,
    minutes    INTEGER     NOT NULL CHECK (minutes > 0),
    note       TEXT,
    logged_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_task_worklogs_task ON task_worklogs(task_id);

COMMIT;
