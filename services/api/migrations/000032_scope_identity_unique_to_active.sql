-- 000032_scope_identity_unique_to_active.sql
-- Scope the email + oidc_sub uniqueness constraints to active (non-deleted)
-- users, matching 000016 (which did this for username). A soft-deleted account
-- (deleted_at set) must no longer reserve its email/oidc_sub: without this the
-- reconcile worker cannot recreate the user (409 USER_IDENTITY_TAKEN) and Paca's
-- own JIT login collides too, permanently locking the user out.
--
-- Index names are kept IDENTICAL (NOT renamed to *_active as 000016 did for
-- username): mapUserUniqueViolation() in
-- services/api/internal/repository/postgres/user_repository.go matches the
-- Postgres unique-violation message by the substrings "uni_users_email" /
-- "uni_users_oidc_sub" to return USER_IDENTITY_TAKEN — renaming would silently
-- break that 409 mapping.
--
-- Migrations here re-run on every API boot with no tracking table, so this must
-- be idempotent (DROP IF EXISTS + CREATE IF NOT EXISTS) and — being a higher
-- number than 000022 — always runs AFTER it, re-scoping the index 000022 would
-- otherwise recreate unscoped.

BEGIN;

DROP INDEX IF EXISTS uni_users_email;

CREATE UNIQUE INDEX IF NOT EXISTS uni_users_email
    ON users (email)
    WHERE email IS NOT NULL AND deleted_at IS NULL;

DROP INDEX IF EXISTS uni_users_oidc_sub;

CREATE UNIQUE INDEX IF NOT EXISTS uni_users_oidc_sub
    ON users (oidc_sub)
    WHERE oidc_sub IS NOT NULL AND deleted_at IS NULL;

COMMIT;
