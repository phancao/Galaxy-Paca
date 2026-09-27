# Galaxy-Paca runbook

Operational notes for the Galaxy deployment of Paca (`tasks.skyplatform.net`),
the fork's prod overlay `deploy/galaxy/docker-compose.galaxy.yml` (ADR-038, T8).

## Stack invocation (prod)

```bash
cd ~/Nexus/Galaxy-Paca
docker compose \
  --env-file deploy/galaxy/.env.galaxy \
  -f deploy/docker-compose.prod.yml \
  -f deploy/galaxy/docker-compose.galaxy.yml \
  up -d --scale ai-agent=0
```

No host ports: the Caddy gateway (`paca-edge`) joins `galaxy_network` (alias
`paca-gateway`); the Cloudflare tunnel terminates TLS and routes
`tasks.skyplatform.net → paca-gateway:80`. Container names are
`galaxy-paca-<service>-1` (the mcp one is `galaxy-paca-mcp`).

> **Bind-mounted `Caddyfile` inode trap:** `deploy/caddy/Caddyfile` is bind
> mounted into `paca-edge`. After editing/pulling it, recreate the container
> (`up -d --force-recreate paca-edge`) — a reload keeps the old inode.

---

## SDD (đã gỡ 27/09/2026)

Cụm SDD (plugin `com.galaxy.sdd`, sidecar `sdd-proxy`, `sdd-server` + Postgres riêng, tuyến `/sdd-api/*`) đã được gỡ khỏi Paca. Việc điều phối nhiệm vụ cho agent chuyển sang app Paperclip (fork, ADR-080). Lịch sử: ADR-038 T6 trong kho Galaxy-Vortex.
