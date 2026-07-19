# Galaxy SDD Coordination Server — central, online, multi-machine.
# Build context = repo root (it reuses ../server/lib classifier + ../client UI).
#
#   docker build -f central/Dockerfile --build-arg VITE_BASE_PATH=/sdd-server .

# ── Stage 1: build the React UI under the gateway sub-path ────────────────────
FROM node:22-alpine AS client-build
WORKDIR /app/client
ARG VITE_BASE_PATH=/sdd-server
ENV VITE_BASE_PATH=$VITE_BASE_PATH
# Central server requires a Vortex OIDC login for the UI.
ARG VITE_OIDC=1
ENV VITE_OIDC=$VITE_OIDC
ARG VITE_OIDC_CLIENT_ID=sdd-server
ENV VITE_OIDC_CLIENT_ID=$VITE_OIDC_CLIENT_ID
COPY client/package.json client/package-lock.json ./
RUN npm ci
COPY client/ ./
RUN npm run build

# ── Stage 2: central server production deps ──────────────────────────────────
FROM node:22-alpine AS central-deps
WORKDIR /app/central
COPY central/package.json central/package-lock.json ./
RUN npm ci --omit=dev

# ── Stage 3: runtime ─────────────────────────────────────────────────────────
FROM node:22-alpine
WORKDIR /app

# central service + its deps
COPY central/ ./central/
COPY --from=central-deps /app/central/node_modules ./central/node_modules/
# the PURE classifier + rules the server reuses (no sqlite pulled in)
COPY server/lib/sdd-classify.js ./server/lib/sdd-classify.js
COPY server/lib/sdd-rules.js ./server/lib/sdd-rules.js
COPY server/config/sdd-rules.json ./server/config/sdd-rules.json
# the built team UI
COPY --from=client-build /app/client/dist ./client/dist/

ENV NODE_ENV=production
EXPOSE 4830
CMD ["node", "central/index.js"]
