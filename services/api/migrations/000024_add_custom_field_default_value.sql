-- 000024_add_custom_field_default_value.sql
-- ADR-040 (advanced task schema, Phase 0): a custom field definition may carry
-- a default value that is applied to a task's custom_fields when the field is
-- otherwise absent. Stored as JSONB so it holds the field's native type
-- (string / number / boolean / array of options).
--
-- Nullable — existing definitions keep no default and are unaffected.

BEGIN;

ALTER TABLE custom_field_definitions
    ADD COLUMN IF NOT EXISTS default_value JSONB;

COMMIT;
