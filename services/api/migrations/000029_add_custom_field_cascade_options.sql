-- 000029_add_custom_field_cascade_options.sql
-- ADR-040: cascading_select field type — a two-level parent→child option tree.
-- Stored as JSONB [{"value": "...", "children": ["...", ...]}, ...] separate
-- from the flat `options` (which stays for select/multi_select).
--
-- Nullable — non-cascading fields keep it NULL and are unaffected.

BEGIN;

ALTER TABLE custom_field_definitions
    ADD COLUMN IF NOT EXISTS cascade_options JSONB;

COMMIT;
