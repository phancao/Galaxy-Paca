-- 000026_add_custom_field_task_type_scope.sql
-- ADR-040 (advanced task schema, Phase 2): scope a custom field to a single
-- task type (Jira-style per-issue-type field configuration).
--
--   task_type_id NULL = the field applies to every task type (current behaviour)
--   task_type_id set  = the field only appears on / is validated for tasks of
--                       that type (a required field then only requires that type)
--
-- Nullable — existing definitions stay project-wide and are unaffected.

BEGIN;

ALTER TABLE custom_field_definitions
    ADD COLUMN IF NOT EXISTS task_type_id UUID REFERENCES task_types(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_custom_fields_task_type ON custom_field_definitions(task_type_id);

COMMIT;
