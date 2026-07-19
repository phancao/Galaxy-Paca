-- 000028_add_task_estimate_version_component.sql
-- ADR-040 (advanced task schema, Phase 2): task-side time tracking + release /
-- component association. Runs after 000027 (which creates versions/components).
--
--   estimate_minutes  original time estimate (logged time = SUM(task_worklogs))
--   version_id        target release (fixVersion), nulled if the version is deleted
--   component_id      owning component, nulled if the component is deleted
--
-- All nullable — existing tasks are unaffected.

BEGIN;

ALTER TABLE tasks
    ADD COLUMN IF NOT EXISTS estimate_minutes INTEGER,
    ADD COLUMN IF NOT EXISTS version_id       UUID REFERENCES versions(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS component_id     UUID REFERENCES components(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_tasks_version   ON tasks(version_id);
CREATE INDEX IF NOT EXISTS idx_tasks_component ON tasks(component_id);

COMMIT;
