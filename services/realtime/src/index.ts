// Realtime service entry point.
//
// Starts the Socket.IO server and the Valkey Pub/Sub subscriber, then blocks
// until a SIGTERM or SIGINT signal triggers a graceful shutdown.
//
// Two separate ioredis clients are created:
//   sessionRedis  — regular R/W client for session data (join/leave/disconnect).
//   subscriber    — client locked into subscriber mode (subscribe/psubscribe only).
//                   ioredis requires a dedicated client for this mode.

import Redis from "ioredis";
import pino from "pino";
import { loadConfig } from "./config.ts";
import { createSocketServer } from "./server.ts";
import { createSubscriber } from "./subscriber.ts";

const config = loadConfig();

const logger = pino({
	level: config.logLevel,
	...(process.env.NODE_ENV !== "production" && {
		transport: { target: "pino-pretty", options: { colorize: true } },
	}),
});

logger.info("starting paca realtime service");

// Session store client — regular command mode.
const sessionRedis = new Redis(config.valkey.url, {
	lazyConnect: false,
	maxRetriesPerRequest: null,
});

sessionRedis.on("connect", () =>
	logger.info("valkey session client connected"),
);
sessionRedis.on("error", (err: unknown) =>
	logger.error({ err }, "valkey session client error"),
);

const { httpServer, io } = createSocketServer(config, sessionRedis, logger);
// One subscriber per tenant. They listen on different Valkey databases, so an
// event published by one workspace is not merely filtered away from another —
// it never reaches the other's connection at all.
const subscribers = config.valkey.tenants.map((t) =>
	createSubscriber(t.tenant, t.url, io, logger),
);

httpServer.listen(config.port, () => {
	logger.info({ port: config.port }, "realtime service listening");
});

// ── Graceful shutdown ────────────────────────────────────────────────────────

function shutdown(signal: string): void {
	logger.info({ signal }, "shutdown signal received");

	// Close Socket.IO first to actively terminate WebSocket connections,
	// then close the HTTP server so it stops accepting new connections.
	io.close(() => {
		httpServer.close(() => {
			subscribers.forEach((s) => s.disconnect());
			sessionRedis.disconnect();
			logger.info("shutdown complete");
			process.exit(0);
		});
	});

	setTimeout(() => {
		logger.warn("forced shutdown after timeout");
		process.exit(1);
	}, 10_000).unref();
}

process.on("SIGTERM", () => shutdown("SIGTERM"));
process.on("SIGINT", () => shutdown("SIGINT"));
