-- 000025_add_status_transitions.sql
-- ADR-040 (advanced task schema, Phase 1): an opt-in workflow transition engine.
--
-- Each row declares an ALLOWED status transition for a project. Enforcement is
-- opt-in per project: while a project has ZERO transition rows, task status may
-- move freely (current behaviour, backward compatible). Once any rows exist, a
-- status change must be permitted by a matching row.
--
--   task_type_id   NULL = applies to every task type,   else scoped to one type
--   from_status_id NULL = allowed from ANY source status, else that source only
--   to_status_id   the destination status (required)
--   required_fields JSONB array of custom-field field_keys that must be set on
--                   the task for the transition to succeed
--
-- Creating a task (initial status) and staying in the same status are always
-- allowed and never consult this table.

BEGIN;

CREATE TABLE IF NOT EXISTS status_transitions (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    task_type_id    UUID        REFERENCES task_types(id) ON DELETE CASCADE,
    from_status_id  UUID        REFERENCES task_statuses(id) ON DELETE CASCADE,
    to_status_id    UUID        NOT NULL REFERENCES task_statuses(id) ON DELETE CASCADE,
    required_fields JSONB       NOT NULL DEFAULT '[]'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_status_transitions_project ON status_transitions(project_id);

-- Prevent duplicate rules (NULLs folded to a sentinel so they compare equal).
CREATE UNIQUE INDEX IF NOT EXISTS uq_status_transition ON status_transitions (
    project_id,
    COALESCE(task_type_id, '00000000-0000-0000-0000-000000000000'::uuid),
    COALESCE(from_status_id, '00000000-0000-0000-0000-000000000000'::uuid),
    to_status_id
);

COMMIT;
