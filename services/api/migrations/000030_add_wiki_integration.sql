-- 000030_add_wiki_integration.sql
-- ADR-042 (Paca Documentation → embedded Galaxy AI Wiki): mapping tables that
-- link Paca aggregates to Wiki aggregates. The Wiki (Outline fork) is the
-- system of record for document content; Paca only stores the association.
--
--   project_wiki_spaces  one row per project: which Wiki Folder (top-level
--                        space) hosts this project's documentation, and in
--                        which Wiki Team (tenant) it lives. Provisioned
--                        lazily via act-as on first Documentation open.
--   task_wiki_links      task ↔ Wiki Record (page) associations powering the
--                        "Linked pages" panel on a task and the "Linked
--                        issues" strip around the embedded editor. url/title
--                        are denormalised for rendering without a Wiki call.
--
-- Wiki ids are UUIDs on the Wiki side but are stored as TEXT: they belong to
-- a foreign system, so no FK semantics or uuid ops apply here.

BEGIN;

CREATE TABLE IF NOT EXISTS project_wiki_spaces (
    project_id      UUID        PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    wiki_folder_id  TEXT        NOT NULL,
    wiki_team_id    TEXT        NOT NULL DEFAULT '',
    wiki_url        TEXT        NOT NULL DEFAULT '',
    created_by      UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS task_wiki_links (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id         UUID        NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    wiki_record_id  TEXT        NOT NULL,
    wiki_url        TEXT        NOT NULL DEFAULT '',
    title           TEXT        NOT NULL DEFAULT '',
    created_by      UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (task_id, wiki_record_id)
);

CREATE INDEX IF NOT EXISTS idx_task_wiki_links_task ON task_wiki_links(task_id);

COMMIT;
