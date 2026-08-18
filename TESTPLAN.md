# Galaxy-Paca ("Galaxy Tasks") — Test Plan

> **Scope & status.** Smoke tests for the Galaxy deployment of Paca
> (`https://tasks.skyplatform.net`). This is an **active WIP fork** — some
> surfaces are deliberately dormant (in-app agents) and a few known defects are
> listed in §B. Tests are ordered by priority: health → SSO auth → project/task
> CRUD → AI invoke → identity sync. Prefer running write tests against a scratch
> project, not live data.
>
> **Note on "OpenProject sync":** Paca is standalone and does **not** sync with
> OpenProject (see `ARCHITECTURE.md` §intro). The equivalent integration surfaces
> exercised below are the **Vortex identity-sync webhook (ADR-040)**, the
> **SDD→Paca bridge**, and the **ChatDock / write-with-AI** platform-AI path.

## Conventions

- **Prod containers** (compose project `galaxy-paca`, names `galaxy-paca-<svc>-1`):
  `galaxy-paca-api-1`, `galaxy-paca-web-1`, `galaxy-paca-realtime-1`,
  `galaxy-paca-postgres-1`, `galaxy-paca-valkey-1`, `galaxy-paca-minio-1`,
  `galaxy-paca-paca-edge-1`, `galaxy-paca-notify-bridge-1`,
  `galaxy-paca-dock-trigger-1`, `galaxy-paca-sdd-proxy-1`, `sdd-server`,
  `sdd-server-postgres`. Confirm the exact list with:
  ```bash
  docker compose -p galaxy-paca ps
  ```
- **API base:** `https://tasks.skyplatform.net/api/v1` (public health at
  `/api/healthz`).
- **Web:** `https://tasks.skyplatform.net`.
- `$TOKEN` below = a valid API key or session access token; `$PID` = a project
  UUID; `$TID` = a task UUID.

---

## 1. API health (P0)

`GET /api/healthz` → `200 {"status":"ok"}` (`health_handler.go:16`).

```bash
curl -fsS https://tasks.skyplatform.net/api/healthz            # {"status":"ok"}
# in-container:
docker exec galaxy-paca-api-1 wget -qO- http://localhost:8080/api/healthz
```
Pass: HTTP 200 + `status:ok`. Fail if non-200 (the compose healthcheck uses the
same probe, `docker-compose.prod.yml`).

## 2. Web SPA loads (P0)

Open `https://tasks.skyplatform.net` → login screen renders ("Sign in with
Vortex" button when SSO on; "Bạn đã đăng xuất khỏi Galaxy Tasks." after logout).
```bash
curl -fsS -o /dev/null -w '%{http_code}\n' https://tasks.skyplatform.net/   # 200
```

## 3. Realtime hub health (P1)

`GET /ws/healthz` via Caddy → 200; socket requires a valid JWT.
```bash
curl -fsS -o /dev/null -w '%{http_code}\n' https://tasks.skyplatform.net/ws/healthz
docker exec galaxy-paca-realtime-1 wget -qO- http://localhost:3001/healthz
```
Deeper: connect a Socket.IO client with `auth.token`; a bad token must be
refused (middleware calls the API `/users/me/global-permissions`,
`realtime/src/server.ts:106-149`). On `join {projectId}` the client should only
receive `event`/`notification` for rooms it is permitted (`permissions.ts`).

## 4. Vortex SSO — OIDC login (P0)

`GET /api/v1/auth/oidc/login` → 302 to the Vortex authorize URL; after consent,
`/auth/oidc/callback` links/JITs the user and sets session cookies
(`galaxyauth_service.go:82-163`).
```bash
curl -fsS -o /dev/null -w '%{http_code} %{redirect_url}\n' \
  "https://tasks.skyplatform.net/api/v1/auth/oidc/login"     # 302 → identity
```
Manual: complete an SSO login in a browser; then `GET /api/v1/users/me` returns
your account. First-time SSO must NOT create a duplicate (directory pre-link).

## 5. Vortex RS256 bearer auth (P0)

A platform-minted RS256 token with an `act_as` naming an `oidc_sub`-linked user
authenticates as that user; an unknown principal is rejected 401
(`galaxyauth/bearer.go:74-110`).
```bash
curl -s -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $RS256_TOKEN" \
  https://tasks.skyplatform.net/api/v1/users/me                # 200 linked / 401 unknown
```
Negative (PACA-C1): a token whose `aud`/scope targets another resource must be
rejected; a read-only Paca scope must fail on a write method
(`bearer.go:134-179`).

## 6. Local login / refresh / logout (P1)

```bash
# login → sets HttpOnly cookies
curl -si -c cj.txt -X POST https://tasks.skyplatform.net/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"...","password":"...","rememberMe":false}'   # 200 + Set-Cookie
curl -si -b cj.txt -X POST https://tasks.skyplatform.net/api/v1/auth/refresh   # 200, rotated
curl -si -b cj.txt -X POST https://tasks.skyplatform.net/api/v1/auth/logout    # 200, family revoked
```
Pass: refresh rotates the pair; after logout, refresh fails (family revoked).

## 7. Project CRUD (P0)

```bash
# create
curl -fsS -X POST https://tasks.skyplatform.net/api/v1/projects \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Smoke Test","task_id_prefix":"SMK"}'             # 201
# list / get / update / delete
curl -fsS -H "Authorization: Bearer $TOKEN" https://tasks.skyplatform.net/api/v1/projects
curl -fsS -H "Authorization: Bearer $TOKEN" https://tasks.skyplatform.net/api/v1/projects/$PID
curl -fsS -X PATCH  .../projects/$PID  -d '{"name":"Smoke Test 2"}' -H ...
curl -fsS -X DELETE .../projects/$PID  -H "Authorization: Bearer $TOKEN"   # 204 (soft-delete)
```
Pass: create needs `projects.create`; delete cascades soft-delete to tasks +
members (commit `3652a22`, PACA1/PACA2). Verify the row is gone from listings but
`deleted_at` is set in Postgres.

## 8. Task CRUD + board (P0)

```bash
curl -fsS -X POST https://tasks.skyplatform.net/api/v1/projects/$PID/tasks \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Smoke task","type_id":"...","status_id":"..."}'   # 201, task_number assigned
curl -fsS .../tasks/$TID                                          # get by id
curl -fsS .../tasks/by-number/SMK-1                               # get by number
curl -fsS -X PATCH  .../tasks/$TID -d '{"title":"Renamed"}'       # update
curl -fsS -X DELETE .../tasks/$TID                                # soft-delete
```
Also: comments (`/{taskId}/activities/comments`), links
(`/{taskId}/links`), worklogs (`/{taskId}/worklogs`), attachments
(initiate/complete upload → `/download-url`). Invalid references must return
4xx, not 500 (commit `d230484`).

## 9. Sprints & views (P1)

Create sprint → add tasks → `POST /sprints/{id}/complete`; board views list +
`PUT /views/{id}/task-positions` (drag-drop persistence). Pass: completing a
sprint moves unfinished tasks per app rules; view positions round-trip.

## 10. Task config: types / statuses / custom fields / transitions (P1)

CRUD `/task-types`, `/task-statuses` (+ `PUT /positions`, set-default),
`/custom-fields` (incl. `cascading_select` cascade options, `000029`),
`/status-transitions`. Pass: custom-field + workflow-transition validation errors
map to 4xx not 500 (commits `10d369a`, `3a7e4c8`).

## 11. Automation workflows (P2)

Create a draft workflow, add nodes/edges/status-rules/status-transitions,
`activate`, `archive`, `revert-to-draft`. Pass: cycle/self-loop/cross-project
edges rejected; activation requires ≥1 node + a determinable done status.

## 12. Documents (P2)

CRUD `/docs/folders` and `/docs`; snapshots list/get; comments; doc file
up/download. Pass: BlockNote content round-trips; snapshot numbers increment.

## 13. AI: write-with-AI (P0 — active AI path)

`POST /api/v1/projects/$PID/tasks/$TID/write-with-ai` with `{title, description}`
→ Markdown text generated via the platform `/ai/v1` proxy under role `paca-ai`
(`agent_handler.go:697-733`, `galaxyai/client.go`).
```bash
curl -s -X POST https://tasks.skyplatform.net/api/v1/projects/$PID/tasks/$TID/write-with-ai \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Design the onboarding flow"}'
```
Pass: 200 with non-empty `text`. **Known defect B1:** when the platform AI is
unconfigured the response is **HTTP 500** (not the documented 503); when the
upstream call fails the client receives 500 with the raw upstream error text.
Verify billing/usage attributes to the requesting user (act_as token).
Config check:
```bash
docker exec galaxy-paca-api-1 printenv | grep -E 'GALAXY_(IDENTITY_URL|AI_ROLE|INTERNAL_SERVICE_SECRET)'
```

## 14. AI: ChatDock event trigger (P1)

Assign a task to (or `@mention`) the service user `galaxy-tasks-agent` in a
project it belongs to → the `dock-trigger` service runs the AgentOps `paca`
assistant as the assigner and comments the result back
(`deploy/galaxy/README.md:81-108`).
```bash
docker logs --tail=50 galaxy-paca-dock-trigger-1        # dispatch / skip-no-oidc_sub lines
docker exec galaxy-paca-dock-trigger-1 printenv | grep -E 'DOCK_TRIGGER_ENABLED|DOCK_AGENT_TRIGGER_USERNAMES'
```
Pass: with `DOCK_TRIGGER_ENABLED=true`, an assignment produces an agent comment;
an actor without an `oidc_sub` is skipped with a log line (at-most-once).

## 15. MCP server tools (P1)

`@paca-ai/paca-mcp` in bearer mode: set `PACA_AUTH_MODE=bearer` +
`PACA_MCP_TOKEN` (RS256), then exercise `list_projects`, `create_task`,
`add_task_comment` (`apps/mcp/src/tools/index.ts`). Pass: writes land in Paca
attributed to the token's `act_as` user; a 401 latches and refuses retries
(`apps/mcp/src/auth.ts`).

## 16. Identity-sync webhook — ADR-040 (P0 — integration surface)

`POST /api/v1/nexus/webhook` with a valid `X-Nexus-Signature`/`X-Nexus-Timestamp`
HMAC over `"<ts>."+body` applies `user.changed`: deprovision (soft-delete +
cut refresh) / restore (`nexus_webhook_handler.go`, `nexussync_service.go`).
```bash
# unsigned/invalid → 401
curl -s -o /dev/null -w '%{http_code}\n' -X POST \
  https://tasks.skyplatform.net/api/v1/nexus/webhook \
  -H 'Content-Type: application/json' -d '{"type":"user.changed","user_id":"x"}'   # 401
docker exec galaxy-paca-api-1 printenv | grep -E 'VORTEX_WEBHOOK_SECRET|NEXUS_WEBHOOK_SECRET'
```
Pass: bad signature 401; a valid deprovision blocks that user's next
refresh/bearer call (access token still valid up to ~15m — documented residual,
`nexussync_service.go:96-108`); replayed `_id` is deduped (200 `{"deduped":...}`);
restore only for `oidc_sub`-linked rows after an identity read-back.

## 17. SDD → Paca bridge & Fleet plugin (P2)

`/sdd-api/*` is session-gated: anonymous → 401; a Paca session → JSON
(`runbook:89-96`).
```bash
curl -s https://tasks.skyplatform.net/sdd-api/team/overview     # {"error":{"code":"UNAUTHENTICATED"…}}
curl -s -o /dev/null -w '%{http_code}\n' \
  'https://tasks.skyplatform.net/plugins/com.galaxy.sdd/assets/remoteEntry.js?v=3'   # 200
docker exec sdd-server printenv | grep PACA_BRIDGE_ENABLED
```
Pass: SDD tasks mirror into Paca as comments; the Fleet view renders (no iframe).

## 18. Admin: users & global roles (P1)

`/admin/users` and `/admin/global-roles` CRUD gated by `users.*` /
`global_roles.*`. Pass: a non-admin gets 403; `assign_role` (PUT
`/admin/users/{id}/global-roles`) respects the grant ceiling (cannot grant
beyond caller's own perms, `authz/ceiling.go`). Verify `is_service=true` accounts
are shown with a "Service" badge and are never deprovisioned by sync.

## 19. API keys (P2)

`POST /users/me/api-keys` (JWT-only) → one-time secret; use it as `X-API-Key`;
`DELETE` revokes. Pass: revoked/expired keys are rejected.

## 20. Plugins subsystem (P2)

`GET /plugins` (public listing); admin install/marketplace/upgrade/delete under
`/admin/plugins`. **B4:** `/plugins/{id}/*` proxy enforces auth inside
`ProxyRequest`, not at the router — add an explicit test that an unauthorized
caller cannot reach a capability-gated plugin route (`router.go:676-678`).

## 21. Notifications (P2)

`GET /users/me/notifications`, mark-read, read-all; the `notify-bridge` service
republishes assign/mention events to the platform inbox.
```bash
docker logs --tail=30 galaxy-paca-notify-bridge-1
```

---

## B. Known defects to assert (do NOT auto-pass) — REPORT ONLY

- **B1** write-with-AI returns HTTP **500** when unconfigured/failed, not the
  documented 503, and leaks upstream error text (`agent_handler.go:698-700,728`).
- **B2** `GET /agents/llm-models` proxies to the retired `ai-agent` service →
  always 500 in Galaxy (`agent_handler.go:763-790`). The in-app agent/conversation
  UI has no runtime in Galaxy.
- **B3** `Access-Control-Allow-Origin: *` on all routes (`router.go:756`).
- **B4** plugin proxy auth is handler-internal, not router-enforced
  (`router.go:676-678`).
- **Docs drift** `ROADMAP.md` marks shipped features (RBAC/SSO/health) as Planned.

## C. Regression suites already in-repo

- Go unit + integration + e2e: `services/api/test/{integration,e2e}` (task,
  project, auth, apikey, admin authz, workflow, plugin, view, attachment…).
  ```bash
  cd services/api && go test ./...
  ```
- Web: `apps/web` Vitest (`npm test`); E2E: `apps/e2e` Playwright.
- MCP: `apps/mcp` build + tests; ai-agent: `services/ai-agent/tests` (pytest).
