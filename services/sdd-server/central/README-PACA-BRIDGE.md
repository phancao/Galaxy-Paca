# Paca Bridge — SDD sensor signals into Paca (ADR-038 T6)

Per **ADR-038 T6** ("SDD converges, telemetry stays outside"), the human task
board moves to **Paca** (the Galaxy task system, `tasks.skyplatform.net`,
in-network gateway alias `paca-gateway`). This coordination server stays what
it is — a **telemetry sensor** (Claude-Code hook ingest, SDD classification,
conflict detection, spec-version tracking) — and now **emits signals into
Paca** instead of being the place where humans manage work.

`central/paca-bridge.js` implements the first increment: two signals, posted
as **task comments** through the Paca REST API.

## Signals

| # | Trigger (sensor side) | Target (Paca side) | Comment |
|---|---|---|---|
| 1 | An **open `shared_core_parallel` conflict** not yet bridged (`sdd_conflicts.bridged_at IS NULL`) | Every **non-done** task in any visible project whose `custom_fields.repo` matches the conflict's repo | `⚠️ SDD sensor: shared-core parallel edit detected on `<conflict_key>` — users: <list>, window 30m. Review before merging.` |
| 2 | A **new `sdd_spec_versions` row** not yet bridged | Tasks whose `custom_fields.spec_doc_id` equals the published `doc_id` | `📄 Spec `<title>` version <version> published (source: <source>).` |

Matching details:

- Paca does **not** support filtering task lists by custom field, so the
  bridge enumerates projects (cached 5 min), reads each project's
  `task-statuses` to derive *non-done* (`category != 'done'`; a task with no
  status counts as non-done), paginates `GET .../tasks` (`page_size=100`) and
  matches **client-side** on `task.custom_fields`.
- `conflict_key` is `"<repo>:<dir>"`; tasks match by *prefix*
  (`conflict_key === repo` or `startsWith(repo + ":")`) so repos containing
  `:` (SSH remotes) match correctly.
- Bookkeeping lives in the sensor DB: nullable `bridged_at` columns on
  `sdd_conflicts` and `sdd_spec_versions` (added idempotently by
  `schema.sql`, which `db.init()` re-applies at every boot). A row is marked
  bridged only after **every** comment for it succeeded; failures are retried
  on the next sweep. Repeat conflicts on the same `conflict_key` within 30
  minutes are marked bridged **without** re-commenting (ingest inserts a
  fresh conflict row per event burst — the suppression stops comment spam).

## How it runs

- `startPacaBridge({db, log})` is called once in `index.js` `start()` after
  `db.init()`. Ingest calls `pacaBridge.nudge()` right after inserting a
  conflict / spec version — synchronous, never throws, never awaited.
- A periodic **sweep** (`PACA_BRIDGE_POLL_MS`, default 30 s) is the source of
  truth; nudges only bring the next sweep forward. Sweeps are serialized and
  coalesced, process at most 20 unbridged rows per kind per pass (oldest
  first), and **skip Paca entirely** when nothing is unbridged.
- The task snapshot is all-or-nothing: if any project/status/task listing
  fails, the sweep aborts without marking anything, so a partial snapshot can
  never permanently miss a match.
- **The bridge can never fail or slow ingest**: every Paca call has a 10 s
  timeout and retries once on 5xx; all bridge work is wrapped in try/catch
  and only logs `err.message`. The API key is sent solely in the `X-API-Key`
  header — never in URLs, never logged.

## Configuration (env)

| Env | Default | Meaning |
|---|---|---|
| `PACA_BRIDGE_ENABLED` | `false` | Master switch (`1`/`true`/`on`/`yes` to enable). Off = bridge is fully inert. |
| `PACA_BASE_URL` | – | Paca API base, e.g. `http://paca-gateway:80` (both stacks join `galaxy_network`). |
| `PACA_API_KEY` | – | API key of the dedicated **sdd-sensor** Paca user (sent as `X-API-Key`). |
| `PACA_BRIDGE_POLL_MS` | `30000` | Sweep interval in ms. |

Enabled without `PACA_BASE_URL`/`PACA_API_KEY` → one warning, bridge stays
inert. Nothing else in the server changes.

## Creating the sdd-sensor user + API key in Paca

The bridge authenticates as a normal Paca user. Use a **dedicated service
user** so comments are attributed to the sensor and the key can be rotated
independently:

1. **Create the user** (Paca admin): *Admin → Users → Create* — username
   `sdd-sensor`, full name e.g. `SDD Sensor`, role `USER` — or via API:
   `POST /api/v1/admin/users` `{"username":"sdd-sensor","password":"<tmp>","full_name":"SDD Sensor","role":"USER"}`.
   New users must change the password on first login
   (`PATCH /api/v1/users/me/password`).
2. **Add it to every project** the bridge should comment in (*Project →
   Members*, or `POST /api/v1/projects/:projectId/members`) with a role that
   grants `tasks.read` + `tasks.write` (comments require `tasks.write`). The
   bridge only sees projects the user is a member of.
3. **Create the API key** while logged in as `sdd-sensor`: *User settings →
   API keys*, or `POST /api/v1/users/me/api-keys` — the raw key is shown
   **once**; store it as `PACA_API_KEY` in the sdd-server deployment env
   (never commit it).
4. For the signals to land anywhere, teams must fill the task custom fields
   the bridge matches on: `repo` (custom field key `repo`) and/or
   `spec_doc_id` — define them per project under *Project → Custom fields*
   (`field_type: text`).

Paca API reference used by the bridge (see
`Galaxy-Paca/docs/api/http-design.md` + `docs/api/task-activity.md`):
envelope `{success,data,request_id}`; `GET /api/v1/projects`,
`GET /api/v1/projects/:pid/task-statuses`, `GET /api/v1/projects/:pid/tasks`
(paginated `page`/`page_size`, max 100),
`POST /api/v1/projects/:pid/tasks/:tid/activities/comments` `{text}`.

## Task board deprecation

The built-in cross-machine task board (`tasks` table, `/api/tasks*`, the
Kanban tab) is **superseded by Paca** under ADR-038. It keeps working for
now; in a **later phase** the sensor's board becomes **read-only** and new
work items live in Paca only. Do **not** build new features on `/api/tasks*`.
No board code was changed in this increment.

## Tests

```
cd central && npm test          # node --test __tests__/*.test.js
node --check central/paca-bridge.js
```

Unit tests (mocked fetch, no network/Postgres) cover repo/spec matching,
non-done derivation, comment formatting, the dedup bookkeeping SQL shape,
client pagination/retry/auth-header behavior, and full sweep runs.
