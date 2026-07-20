-- 000031_drop_native_docs.sql
-- ADR-042 Stage 6: the native document feature is removed — project
-- documentation lives in the Galaxy AI Wiki (see 000030 for the mapping
-- tables that replaced it). All four tables were verified empty in
-- production before this migration shipped (0 documents / folders /
-- snapshots / activities); the drop is therefore data-lossless.
--
-- Order respects FKs (children first); CASCADE guards stray dependents.

BEGIN;

DROP TABLE IF EXISTS doc_activities CASCADE;
DROP TABLE IF EXISTS doc_snapshots CASCADE;
DROP TABLE IF EXISTS documents CASCADE;
DROP TABLE IF EXISTS doc_folders CASCADE;

COMMIT;
