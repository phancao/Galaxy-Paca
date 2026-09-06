// Valkey Pub/Sub subscriber.
//
// The API publishes every real-time event to the "paca.events" channel as
// JSON-serialised messages with the shape:
//
//   { "type": "<event-type>", "payload": { ...event-specific fields... } }
//
// Routing
// -------
// Events are routed to namespace-scoped Socket.IO rooms:
//
//   task.*     events  →  project:<projectId>:tasks
//   workflow.* events  →  project:<projectId>:workflows (except
//                          workflow.assigned, which is task-scoped)
//
// Sockets are pre-placed in the correct rooms at join time based on their
// project permissions.  No per-message permission checks are needed here —
// if a socket is in the room it is authorised to receive the event.
//
// Events without a project_id, or with an unrecognised type prefix, are dropped.

import Redis from "ioredis";
import type { Logger } from "pino";
import type { Server } from "socket.io";
import {
	eventNamespace,
	projectRoomName,
	userNotificationRoom,
} from "./permissions.ts";

// CHANNEL is the Valkey Pub/Sub channel the API publishes to.
// Must stay in sync with events.ChannelRealtime in services/api.
const CHANNEL = "paca.events";

interface RealtimeMessage {
	type: string;
	payload?: unknown;
	data?: unknown;
}

// createSubscriber connects a dedicated ioredis client in subscriber mode,
// subscribes to CHANNEL, and wires incoming messages to Socket.IO rooms.
// Returns the ioredis client so the caller can disconnect it on shutdown.
// `tenant` is the workspace this connection speaks for. Every event that
// arrives on it belongs to that workspace and nowhere else, because it is a
// different Valkey database from every other tenant's — the tenant is not
// read from the message, it is a property of the socket the message came in
// on, and therefore cannot be spoofed by a payload.
export function createSubscriber(
	tenant: string,
	valkeyUrl: string,
	io: Server,
	logger: Logger,
): Redis {
	// A separate client is required because ioredis clients in subscriber mode
	// cannot issue regular commands.
	const client = new Redis(valkeyUrl, {
		autoResubscribe: true,
		lazyConnect: false,
		maxRetriesPerRequest: null,
	});

	client.on("connect", () => {
		logger.info({ tenant }, "valkey subscriber connected");
	});

	client.on("reconnecting", () => {
		logger.warn({ tenant }, "valkey subscriber reconnecting");
	});

	client.on("error", (err: unknown) => {
		logger.error({ tenant, err }, "valkey subscriber error");
	});

	client.subscribe(CHANNEL, (err, count) => {
		if (err) {
			logger.error({ tenant, err }, `failed to subscribe to ${CHANNEL}`);
		} else {
			logger.info(
				{ tenant, channel: CHANNEL, subscriptions: count },
				"subscribed to valkey channel",
			);
		}
	});

	client.on("message", (_channel: string, raw: string) => {
		let msg: RealtimeMessage;
		try {
			msg = JSON.parse(raw) as RealtimeMessage;
		} catch (err) {
			logger.warn({ err, raw }, "failed to parse valkey message — skipping");
			return;
		}

		routeEvent(tenant, io, msg, logger);
	});

	return client;
}

function routeEvent(
	tenant: string,
	io: Server,
	msg: RealtimeMessage,
	logger: Logger,
): void {
	const { type } = msg;
	const payload = eventPayload(msg);
	if (!payload) {
		logger.debug({ type }, "event has no payload — skipped");
		return;
	}

	// notification.* events are routed to a user-specific room.
	if (type.startsWith("notification.")) {
		const recipientUserId = payload.recipient_user_id;
		if (typeof recipientUserId !== "string" || !recipientUserId) {
			logger.debug(
				{ type },
				"notification event has no recipient_user_id — skipped",
			);
			return;
		}
		const room = userNotificationRoom(tenant, recipientUserId);
		logger.debug({ tenant, type, room }, "routing notification to user room");
		io.to(room).emit("notification", { type, payload });
		return;
	}

	// Only project-scoped events are forwarded.
	const projectId = payload.project_id;
	if (typeof projectId !== "string" || !projectId) {
		logger.debug({ type }, "event has no project scope — skipped");
		return;
	}

	// Resolve the namespace room from the event type prefix.
	const ns = eventNamespace(type);
	if (!ns) {
		logger.debug({ type }, "unknown event namespace — skipped");
		return;
	}

	const room = projectRoomName(tenant, projectId, ns);
	logger.debug({ tenant, type, room }, "routing event to room");
	io.to(room).emit("event", { type, payload });
}

function eventPayload(msg: RealtimeMessage): Record<string, unknown> | null {
	if (msg.payload && typeof msg.payload === "object") {
		return msg.payload as Record<string, unknown>;
	}
	if (msg.data && typeof msg.data === "object") {
		return msg.data as Record<string, unknown>;
	}
	return null;
}
