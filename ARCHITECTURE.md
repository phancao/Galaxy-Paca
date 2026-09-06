# Galaxy-Paca ("Galaxy Tasks") — Architecture

> **Status: living document for a work-in-progress fork.** Verified against
> `galaxy-main` on 2026-08-18 — that is the branch production runs, not
> `master`, and not the `feat/efficiency-dashboard` this file was first written
> against. The upstream `README.md` / `ROADMAP.md` are stale relative to the
> Galaxy fork (they still mark RBAC, SSO/OIDC and health endpoints as
> "Planned/Phase 3" although the fork has already shipped them). Where the code
> and the prose docs disagree, this file follows the **code** and flags the gap.
> All claims are grounded in `file:line`.
>
> **Important correction to a common assumption:** Paca does **NOT** wrap or
> orchestrate OpenProject. A whole-repo search finds the string only in a test
> helper name (`services/api/internal/repository/postgres/project_repository_test.go:14`
> `openProjectRepoTestDB` — "open [the] project repo test DB", unrelated).
> Paca is a **standalone, self-hosted PM platform** (a fork of the Apache-2.0
> `Paca-AI/paca`, tracking ~`v0.9.7`) that carries its own task/project/sprint
> data model in Postgres. It is the successor that the AI Project Manager (PM)
> and Bugbase migrate **into**, not a façade over another PM engine.

---

## 1. Purpose

Paca is an **AI-native project-management platform** where AI agents and humans
are equal members of the same Scrum/Scrumban team (`README.md:38-77`). In the
Galaxy deployment it is branded **"Galaxy Tasks"** and served at
`https://tasks.skyplatform.net` (`deploy/galaxy/README.md:1-9`,
`apps/web/src/components/auth/login/LoginFormPanel.tsx:68`). It provides:

- Projects, backlogs, sprints, a Scrumban board, task types/statuses, custom
  fields, task links, worklogs (time tracking), versions/components.
- Per-project living documents (BlockNote editor, snapshots, comments).
- Automation workflows (task-dependency graphs, status-rule reassignment).
- A WASM plugin system (SDD Fleet, Analytics, GitHub, BDD, Checklist…).
- AI-agent collaboration (see §8) and an MCP server for external agents (§7).

---

## 2. Tech stack

| Layer | Technology | Evidence |
|:--|:--|:--|
| Core API | Go 1.26, `go-chi/v5` router | `services/api/go.mod:1-3`, `router/router.go:14` |
| Persistence | PostgreSQL via `jmoiron/sqlx`; SQL migrations embedded | `go.mod`, `services/api/migrations/` |
| Cache / bus | Valkey/Redis (`redis/go-redis/v9`) — cache + pub/sub | `go.mod`, `platform/cache`, `platform/messaging` |
| Object store | S3 / MinIO (`aws-sdk-go-v2`) | `platform/storage/s3.go`, `config.go:139-152` |
| Plugins | WASM sandbox via `tetratelabs/wazero` | `platform/plugin/runtime.go` |
| Auth tokens | `golang-jwt/v5` (HS256 sessions, RS256 platform bearer) | `platform/token`, `middleware/galaxy_bearer.go` |
| Web SPA | React 19, Vite, TanStack Router/Query, BlockNote, Module Federation, Socket.IO client | `apps/web/package.json` |
| Realtime | Bun + Socket.IO 4 (`paca-realtime`) | `services/realtime/package.json` |
| MCP server | Node, `@modelcontextprotocol/sdk` 1.26, stdio | `apps/mcp/package.json`, `apps/mcp/src/index.ts` |
| In-app agent (retired) | Python FastAPI + OpenHands SDK | `services/ai-agent/pyproject.toml` |
| Gateway | Caddy 2 | `deploy/caddy/Caddyfile` |

---

## 3. Component / repository map

```
services/
  api/            Go backend (330 .go files) — the system of record
    cmd/api/main.go            entrypoint
    internal/
      bootstrap/               DI wiring (app.go), providers
      config/                  env → typed Config (config.go, load.go)
      domain/                  entities + contracts (user, project, task,
                               sprint, agent, workflow, version, component,
                               worklog, apikey, globalrole, auth, notification…)
      service/                 business logic (+ cached_* Valkey wrappers)
      repository/postgres/     sqlx repositories
      transport/http/          router, handler, dto, middleware, presenter
      platform/                authz, cache, database, oidc, plugin (wazero),
                               token, storage, messaging, secret, galaxyai
      worker/                  Valkey stream consumers (activity, notification,
                               workflow, plugin_event, doc_activity)
      events/, pkg/mention/
    migrations/                32 SQL migrations (embedded)
  realtime/       Bun + Socket.IO event hub (port 3001)
  ai-agent/       Python OpenHands sandbox runner  [RETIRED in Galaxy — §8]
  agent-server/   Dockerfile only: OpenHands sandbox base image [RETIRED]
  sdd-server/     Vendored Galaxy SDD Coordination Server (Node)
apps/
  web/            React 19 SPA (served via Caddy)
  mcp/            @paca-ai/paca-mcp npm MCP server (~73 tools)
  e2e/            Playwright end-to-end suite
deploy/
  docker-compose.{dev,e2e,prod}.yml
  caddy/Caddyfile
  galaxy/         Galaxy production overlay + bridges + native plugins
docs/, skills/
```

The Go API follows a clean/hexagonal layout: `domain` (pure entities+ports) →
`service` (use-cases) → `repository/postgres` (adapters) → `transport/http`
(delivery), with cross-cutting concerns in `platform`.

---

## 4. Runtime topology (Galaxy prod) & gateway routes

The Cloudflare tunnel terminates TLS and forwards `tasks.skyplatform.net` →
`paca-gateway:80` (Caddy `paca-edge`) on the shared `galaxy_network`; no host
ports are published (`deploy/galaxy/docker-compose.galaxy.yml:13-31,306-349`).
Caddy fans out (`deploy/caddy/Caddyfile`):

| Path | Upstream | Purpose |
|:--|:--|:--|
| `/api/*` | `api:8080` | Go REST API | (`Caddyfile:196`) |
| `/sdd-api/*` | `sdd-proxy:8791` | SDD Fleet read proxy (session-gated) | (`:192`) |
| `/ws/*` | `realtime:3001` | Socket.IO | (`:102`) |
| `/storage/*` | `minio:9000` | object storage / presigned uploads | (`:90`) |
| `/plugins/*`, `/plugins-mcp/*` | Caddy file server | plugin frontend + MCP bundles | (`:115,139`) |
| `/dock.js`, `/api/{agentops,identity,notify,calendar,email,chat,pm-mcp}*` | `vortex-gateway` | ChatDock bridge to the platform | (`:52,149-174`) |
| `/` (fallback) | `web:3000` | React SPA | (`:204`) |

---

## 5. Data model

32 migrations, `services/api/migrations/000001_init.sql` …
`000032_scope_identity_unique_to_active.sql`, re-applied idempotently on
startup (`platform/database/migrations.go`). **Single-tenant per deployment:**
there is **no `tenant_id`/`org_id` column anywhere** — a repo-wide grep of the
migrations returns zero matches. Isolation is per-**project** via `project_id`
FKs; multi-tenancy is achieved by deploying separate containers/DBs, one Paca
behind one Vortex tenant (linked through `users.oidc_sub`, `000022`).

**Tables by domain:**

- **Users / auth:** `global_roles` (JSONB `permissions`), `users`
  (`password_hash`, `role_id`, `must_change_password`, `deleted_at`,
  `email`+`oidc_sub` for SSO `000022`, `is_service` for bridge/agent accounts
  `000023`), `api_keys` (`key_hash`, `key_prefix`, `expires_at`, `revoked_at`).
- **Projects / membership:** `projects` (`task_id_prefix`, `settings`,
  `is_public`, `deleted_at`), `project_roles` (per-project or template),
  `project_members` — **polymorphic** `member_type ∈ human|agent` with
  `user_id` or `agent_id` (`000008`); this is the actor most audit columns FK to.
- **Tasks & config:** `tasks` (`task_number`, `description` JSONB, `importance`,
  `custom_fields` JSONB, `tags`, `parent_task_id`, `story_points`,
  `estimate_minutes`, `version_id`, `component_id`, `deleted_at`), `task_types`,
  `task_statuses` (`category`, `position`), `task_counters`, `task_assignees`
  (M2M, replaced single assignee `000021`), `task_links`, `task_attachments`,
  `task_activities` (comment+change log), `task_worklogs` (`minutes`,
  `logged_at`, `000027`), `custom_field_definitions` (`field_type`, `options`,
  `default_value`, `cascade_options`), `status_transitions` (allowed-transition
  rules, `required_fields`).
- **Releases:** `versions` (fixVersion), `components` (`lead_member_id`).
- **Sprints / views:** `sprints`, `sprint_views` (table/board/roadmap/plugin),
  `view_task_positions` (manual ordering).
- **Docs:** `doc_folders` (self-ref tree), `documents` (BlockNote JSONB),
  `doc_snapshots` (trigger-numbered), `doc_activities`.
- **AI agents:** `agents` (`llm_provider/model/base_url`, encrypted
  `llm_api_key_secret`, `system_prompt`), `agent_mcp_servers`, `agent_skills`,
  `agent_environment_variables` (encrypted), `agent_chat_sessions`,
  `agent_conversations` (`trigger_type`, `status`, sandbox `container_id`),
  `agent_conversation_events`.
- **Automation:** `workflows`, `workflow_nodes`, `workflow_edges`,
  `workflow_status_rules`, `workflow_status_transitions`.
- **Plugins:** `plugins` (`manifest` JSONB, `enabled`), `plugin_extension_settings`.
- **Other:** `notifications`, `files` (S3 object registry).
- **Removed/legacy:** GitHub tables (`000007`) and checklist tables (`000005`)
  were dropped from core and moved into plugins.

Soft-delete (`deleted_at`) is used on users, projects, project_members, tasks,
agents, workflows, documents and both activity logs; uniqueness constraints are
scoped to active rows.

---

## 6. Feature / endpoint inventory (REST)

All routes are under `/api/v1` and defined in
`services/api/internal/transport/http/router/router.go`. Auth is cookie/JWT or
API key (`Authn`); most write routes are gated by `RequirePermissions` in the
global or `projectId` scope. Public-project reads use
`RequirePublicProjectOrPermissions`.

| Feature | Method + path | Notes |
|:--|:--|:--|
| Health | `GET /api/healthz` | public, `{"status":"ok"}` (`health_handler.go:16`) |
| Login / refresh / logout | `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout` | HttpOnly cookie session (`router.go:73-76`) |
| Auth config | `GET /auth/config` | advertises SSO button + dock src |
| OIDC SSO | `GET /auth/oidc/login`, `GET /auth/oidc/callback` | only if `OIDC_ISSUER` set (`:79-82`) |
| Identity-sync webhook | `POST /nexus/webhook` | HMAC-authenticated, only if secret set (`:90-92`) |
| Me | `GET/PATCH /users/me`, `PATCH /users/me/password`, `GET /users/me/global-permissions` | (`:95-124`) |
| API keys | `GET/POST /users/me/api-keys`, `DELETE /users/me/api-keys/{keyId}` | JWT-only (`:108-114`) |
| Notifications | `GET /users/me/notifications`, `PATCH …/{id}/read`, `POST …/read-all` | (`:118-122`) |
| Admin users | `GET/POST /admin/users`, `GET/PATCH/DELETE /admin/users/{userId}`, `PATCH …/password` | `users.*` perms (`:132-143`) |
| Admin global roles | `GET/POST /admin/global-roles`, `PATCH/DELETE …/{roleId}`, `PUT /admin/users/{userId}/global-roles` | (`:146-155`) |
| Projects | `GET/POST /projects`, `GET/PATCH/DELETE /projects/{projectId}` | create needs `projects.create` (`:159-189`) |
| Members / roles | `…/members` CRUD + `GET /members/me/permissions`; `…/roles` CRUD | (`:192-218`) |
| Task types / statuses | `…/task-types` CRUD + set-default; `…/task-statuses` CRUD + `PUT /positions` + set-default | (`:221-253`) |
| Automation workflows | `…/workflows` CRUD + activate/archive/revert + nodes/edges/status-rules/status-transitions | (`:256-297`) |
| Sprints | `…/sprints` CRUD + `POST /{sprintId}/complete` | (`:300-317`) |
| Views (board) | `…/views` CRUD + `PUT /positions`; `…/{viewId}/task-positions` list/bulk-move/move | (`:320-346`) |
| Tasks | `…/tasks` CRUD, `GET /by-number/{n}`, `GET /{taskId}` | (`:349-367`) |
| Write-with-AI | `POST /tasks/{taskId}/write-with-ai` | one-shot platform AI (§8); see bug B1 (`:369-372`) |
| Task activities | `…/{taskId}/activities` list; comments add/update/delete | (`:375-386`) |
| Task links | `…/{taskId}/links` list/create/delete | (`:389-398`) |
| Worklogs | `…/{taskId}/worklogs` list/create/delete; `GET …/projects/{id}/worklogs` (project-wide, efficiency report) | (`:401-411,466-471`) |
| Attachments | `…/{taskId}/attachments` list + initiate/complete-upload + download-url + delete | (`:423-438`) |
| Custom fields | `…/custom-fields` CRUD | (`:442-457`) |
| Copy config | `POST /projects/{id}/copy-config` | clone schema from another project (`:460-461`) |
| Status transitions | `…/status-transitions` list/create/delete | (`:474-483`) |
| Versions / components | `…/versions` CRUD; `…/components` CRUD | (`:486-515`) |
| Docs | `…/docs/folders` CRUD; `…/docs` CRUD; snapshots; activities; comments; file up/download | (`:518-590`) |
| Agents (in-app) | `…/agents` CRUD + mcp-servers/skills/env-vars/chat-sessions | RETIRED runtime, see §8 (`:593-644`) |
| LLM models | `GET /agents/llm-models` | proxies to ai-agent; **dead in Galaxy**, bug B2 (`:172`) |
| Skill templates | `GET /agents/skill-templates` | static catalog (`:173`) |
| Conversations | `…/conversations` list/get/events/stop/pause/heartbeat/messages | in-app agent, retired (`:647-664`) |
| Plugins | `GET /plugins`; `ANY /plugins/{id}/*` (proxy); `admin/plugins` install/marketplace/upgrade/delete; `admin/plugin-extension-settings` | (`:668-699`) |

---

## 7. MCP server (`@paca-ai/paca-mcp`)

`apps/mcp` publishes ~73 stdio tools that mirror the REST surface
(`apps/mcp/src/tools/index.ts:51-69`): `list/get/create/update/delete_project`,
the full `*_task*` family, sprints (`…_sprint` + `complete_sprint`),
filesystem-style docs (`list/read/write/delete/move_doc`), members & roles,
task types/statuses, views + custom fields, attachments, task
activities/comments, task links, workflows, doc activities.

**Auth** (`apps/mcp/src/auth.ts`) has two modes via `PACA_AUTH_MODE`:
- `apikey` (upstream default): `X-API-Key: $PACA_API_KEY` (+ optional
  `X-Agent-ID`).
- `bearer` (Galaxy fork, ADR-038): a single RS256 platform token in
  `Authorization: Bearer $PACA_MCP_TOKEN`; the acting principal is the signed
  `act_as` claim, `act_as_agent` for attribution — header impersonation is
  dropped because the Galaxy API rejects it. A 401 latches and refuses retries.

In the Galaxy ChatDock this MCP is the `galaxy-paca-mcp` server exposing the
`paca_*` tools to the AgentOps `paca` assistant (`deploy/galaxy/README.md:88-90`).

---

## 8. AI-agent / registry layer

Paca has had **two AI eras**; both remain in the tree but only one runs in Galaxy.

**(a) Legacy in-app agents — RETIRED in Galaxy (source kept, thin-fork).**
The `agents/agent_*` tables, the `/agents` + `/conversations` REST surface, the
Python `services/ai-agent` OpenHands runner and `services/agent-server` sandbox
image implement in-app agents that pick up tasks and run in isolated Docker
sandboxes. In the Galaxy overlay these are gated behind the
`retired-use-chatdock` Compose profile so a default `up` never starts them
(`docker-compose.galaxy.yml:109-178`, `deploy/galaxy/README.md:11,81-108`).

**(b) Active: platform AI via the ChatDock + one-shot write-with-AI.**

- **Registry model — the `paca-ai` role.** The AI capability is a **role name**,
  not a model. `galaxyai.Client.WriteDescription` sends `"model": c.role`
  (default `"paca-ai"`) to the Vortex `/ai/v1/chat/completions` proxy
  (`platform/galaxyai/client.go:36-50,124-130`); the Vortex identity service
  resolves that role to a concrete model via its `ai_role_assignments` table
  (rebindable in `/nexus/admin → AI Models` with no Paca redeploy —
  `deploy/galaxy/README.md:116`). Configured via `GALAXY_AI_ROLE`
  (`config/load.go:269`, default `paca-ai`).
- **Write-with-AI** (`POST /tasks/{taskId}/write-with-ai`,
  `agent_handler.go:697-733`): mints a short-lived, **non-privileged** `act_as`
  token at identity `/internal/mint-service-token` (authenticated with
  `X-Service-Secret` = platform `INTERNAL_SERVICE_SECRET`), naming the
  requesting user so usage/billing attributes to them
  (`galaxyai/client.go:60-95`), then calls the chat proxy. Disabled → error when
  `GALAXY_IDENTITY_URL`/secret are unset.
- **ChatDock event trigger** (`deploy/galaxy/README.md:81-108`): the
  `dock-trigger` service tails the task-assignment/mention Valkey streams; when a
  task is assigned to or `@mentions` the service user `galaxy-tasks-agent`, it
  mints an RS256 token **as the assigner/mentioner** (`sub = users.oidc_sub`) and
  runs the AgentOps `paca` assistant (react-agent, 23 `paca_*` + `wiki_*` tools),
  which works the task and always comments the result back.
- **Skills:** the three PM analyst skills (`skills/galaxy-sprint-health`,
  `galaxy-triage`, `galaxy-estimate`) were ported into the AgentOps catalog; the
  copies in `skills/` remain as reference for the dormant in-app path.

---

## 9. Auth & RBAC

**Local sessions.** Username/password → HS256 JWT access/refresh pair in
HttpOnly cookies (access `SameSite=Lax` 15m, refresh `SameSite=Strict`,
168h persistent / 24h ephemeral), refresh rotation with family revocation and a
Redis refresh store (`auth_handler.go:78-163`, `config/load.go:30-41`).
`must_change_password` blocks all but the password-change route
(`middleware/must_change_password.go`, `router.go:98-102`).

**Vortex SSO (ADR-038).** Two independent switches:
- **OIDC login** (`OIDC_ISSUER`): find-by-`oidc_sub` → link active account by
  email → optional JIT provision with an unusable random password + default role
  (`service/galaxyauth/galaxyauth_service.go:82-163`).
- **RS256 trusted-issuer bearer** (`GALAXY_TRUSTED_ISSUER`): tokens signed by the
  Vortex JWKS are accepted platform-wide; the effective principal is the
  `act_as` claim (falling back to `sub`), resolved to a local user via
  `users.oidc_sub` — **never auto-provisioned on this path**; `act_as_agent` is
  recorded for attribution only and grants nothing beyond the principal
  (`middleware/galaxy_bearer.go`, `service/galaxyauth/bearer.go:74-110`).
  Hardening (PACA-C1): audience enforcement (`GALAXY_BEARER_AUDIENCE`) and
  resource-scope enforcement (`GALAXY_RESOURCE_SCOPE_PREFIX`, default
  `mcp:paca:`) reject foreign-resource and read-only-token-on-write cases
  (`bearer.go:134-179`, wired `bootstrap/app.go:354-359`). Header impersonation
  (`X-Agent-ID`) is off by default (`config/config.go:242-246`).

**RBAC (local, not OpenFGA).** Permission keys (`platform/authz/permissions.go`)
like `tasks.read/write`, `projects.*`, `agents.*`, with `*` and `prefix.*`
wildcards. Built-in global roles `SUPER_ADMIN`/`ADMIN`/`USER` and project roles
`PROJECT_OWNER/MANAGER/MEMBER/VIEWER` (`platform/authz/defaults.go`). Effective
permissions = legacy role ∪ persisted global role ∪ project role, with a
**grant ceiling** (you cannot create/assign a role broader than your own) and a
parallel **agent** permission path (`platform/authz/ceiling.go`).

**ADR-040 identity lifecycle.** `POST /nexus/webhook` verifies an HMAC-SHA256
signature over `"<ts>."+body` (±5m clock drift, in-process replay dedup) and
applies `user.changed`: non-active/deleted → soft-delete the mirror (cutting the
refresh path and per-request bearer resolution); active → restore, but only for
`oidc_sub`-linked rows and only after a read-back confirms Vortex still reports
the user active (`handler/nexus_webhook_handler.go`,
`service/nexussync/nexussync_service.go`). **Residual exposure:** a
already-issued stateless access token cannot be revoked and survives up to
`JWT_ACCESS_TTL` (~15m) — documented tradeoff (`nexussync_service.go:96-108`).

---

## 10. Multi-tenant

Single-tenant per deployment (see §5). The Galaxy install runs **one** Paca
instance for Vortex tenant `galaxy`; all Vortex `galaxy` users are pre-linked
into Paca (`email`+`oidc_sub`) by an out-of-band reconcile job
(`galaxy-app-admin-reconcile` → `reconcile/paca_user_sync.py`, in the
Galaxy-Authz repo — **not** this stack), hard-guarded to the `galaxy` tenant and
never touching `is_service=true` accounts (`deploy/galaxy/README.md:44-64`).
**Amended 07/09/2026 — one process, one database per tenant.** `PACA_TENANTS`
lists the tenants a single API serves; each gets its own Postgres database,
Valkey logical database and bucket, and the primary keeps the bare
`DATABASE_URL`/`REDIS_URL`/`STORAGE_BUCKET` it already runs on. Requests route
by the `tenant` claim, and every tenant graph re-verifies it, so a session
cannot cross workspaces. See ADR-038 T7.

Instance-per-tenant is still supported (`GATEWAY_NETWORK_ALIAS` →
`paca-<tenant>-gateway`, `deploy/galaxy/tenant-template/`) for a tenant that
needs its own process-level blast radius.

⚠️ The out-of-band reconcile above still pre-links only the `galaxy` tenant.
Users of the other tenants are created on first login by
`OIDC_AUTO_CREATE_USERS`, which is how they get an account at all — extending
the reconcile to the remaining tenants is a separate change in Galaxy-Authz.

---

## 11. Migration status — AI Project Manager / Bugbase → "Galaxy Tasks"

Paca is positioned as the **task/PM successor**; the migration is largely
**inbound and staged**, and most bridge code lives in the *source* repos, not
here. Evidence from this repo:

- **Service accounts as integration seams:** `pm-bridge`, `support-bridge`,
  `sdd-sensor`, `galaxy-tasks-agent` are `is_service=true` accounts that external
  systems push through with admin API keys
  (`deploy/galaxy/README.md:52`, `service/user/user_service_test.go:648`). The
  PM/Bugbase-side bridge implementations are external.
- **SDD Coordination Server fully absorbed:** vendored into
  `services/sdd-server` and surfaced as the native `com.galaxy.sdd` plugin; the
  standalone dashboard is decommissioned and returns a `410` "moved to Galaxy
  Tasks" page (`docs/runbooks/galaxy-paca-runbook.md:30-121`,
  `docker-compose.galaxy.yml:351-415`).
- **PM AI features ported:** the three PM analyst prompts became Paca skills /
  AgentOps `paca` skills (§8), and PM's efficiency/velocity reporting has since
  **shipped**: `apps/web/src/routes/_authenticated/projects/$projectId/efficiency/index.tsx`
  is on `galaxy-main`, backed by the worklog domain
  (`services/api/internal/domain/worklog/entity.go`). The branch this paragraph
  was first written against, `feat/efficiency-dashboard`, no longer exists.
- **Product framing:** the login screen and runbooks call the product "Galaxy
  Tasks" (`LoginFormPanel.tsx:68`, `runbook`).

**Open/WIP:** two feature branches remain — `feature/mcp-bearer` and
`feature/paca-ai-role`. The others this section once listed (`feat/adr040-*`,
`feat/sdd-*`, `feat/efficiency-dashboard`) have been deleted from the remote,
their work merged or abandoned; checked 2026-08-18. There is no in-repo Bugbase data-importer; that
migration path is external and not yet represented here.

---

## 12. Deploy

Prod invocation (`docs/runbooks/galaxy-paca-runbook.md:5-27`):

```bash
cd ~/Nexus/Galaxy-Paca && git pull
docker compose \
  --env-file deploy/galaxy/.env.galaxy \
  -f deploy/docker-compose.prod.yml \
  -f deploy/galaxy/docker-compose.galaxy.yml \
  up -d
```

- Compose project name `galaxy-paca`; container names `galaxy-paca-<service>-1`
  (`docker-compose.galaxy.yml:32`, runbook).
- **Active services:** `postgres`, `valkey`, `minio`, `api`, `web`, `realtime`,
  `paca-edge` (Caddy), `notify-bridge` (Paca events → platform notify inbox),
  `dock-trigger` (task events → ChatDock agent), `sdd-proxy`, `sdd-server`,
  `sdd-server-postgres`, `db-backup`.
- **Profile-gated / retired:** `ai-agent`, `socket-proxy` (profile
  `retired-use-chatdock`); the base `gateway` service is disabled in favour of
  `paca-edge` to avoid a `galaxy_network` alias collision with `vortex-gateway`
  (`docker-compose.galaxy.yml:298-349`).
- Memory limits on every service; nightly Postgres backup to
  `/backup/paca-postgres`. Health check: `GET /api/healthz`.
- Key env (`config/load.go`): required `JWT_SECRET`, `DATABASE_URL`,
  `REDIS_URL`, `ADMIN_USERNAME/PASSWORD`, `STORAGE_ACCESS_KEY_ID/SECRET`.
  Galaxy-specific: `OIDC_*`, `GALAXY_TRUSTED_ISSUER[_CLAIMS]`,
  `GALAXY_IDENTITY_URL`, `GALAXY_INTERNAL_SERVICE_SECRET`, `GALAXY_AI_ROLE`,
  `GALAXY_BEARER_AUDIENCE`, `GALAXY_RESOURCE_SCOPE_PREFIX`, `GALAXY_DOCK_SRC`,
  `VORTEX_WEBHOOK_SECRET`.

---

## 13. Known gaps / issues (documentation flags — see TESTPLAN.md)

- **B1 — write-with-AI status mismatch.** `agent_handler.go:698-700` returns
  `CodeInternalError` → **HTTP 500** ("AI writing is not configured"), but the
  handler comment and config docs say it "returns 503". The upstream failure
  path (`:728`) also returns 500 with `"AI writing failed: "+err.Error()`,
  echoing upstream error text to the client. There is no 503 code in `apierr`.
- **B2 — dead LLM-models endpoint in Galaxy.** `GET /agents/llm-models` proxies
  to the `ai-agent` service (`agent_handler.go:769`), which is retired/scaled-0
  in the Galaxy overlay → the call always fails with 500. The whole in-app agent
  UI surface has no runtime backing it in Galaxy (by design, but user-visible).
- **B3 — permissive CORS.** `router.go:756` sets
  `Access-Control-Allow-Origin: *` on all routes with a self-noted "tighten in
  production". Cookie sessions are largely protected (SameSite + no credentialed
  `*`), but it is a hardening gap for bearer/API-key callers.
- **B4 — plugin proxy auth is handler-internal.** `router.go:676-678` registers
  `/plugins/{pluginId}/*` with no router-level auth; correctness depends entirely
  on `PluginHandler.ProxyRequest`. Worth an explicit authz test.
- **Docs drift.** `ROADMAP.md` marks RBAC, SSO/OIDC and health endpoints as
  Planned/Phase 3 though the fork ships them; the upstream `README.md` describes
  the generic OSS product, not the Galaxy topology.
