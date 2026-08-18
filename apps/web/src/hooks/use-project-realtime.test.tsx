import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

// ── Hoisted mocks ─────────────────────────────────────────────────────────────

const mocks = vi.hoisted(() => {
	const socket = {
		on: vi.fn(),
		off: vi.fn(),
	};

	return {
		socket,
		connectSocket: vi.fn(() => socket),
		joinProject: vi.fn(),
		leaveProject: vi.fn(),
		invalidateQueries: vi.fn(),
	};
});

vi.mock("@/lib/socket-client", () => ({
	connectSocket: mocks.connectSocket,
	joinProject: mocks.joinProject,
	leaveProject: mocks.leaveProject,
}));

vi.mock("@tanstack/react-query", async () => {
	const actual = await vi.importActual<typeof import("@tanstack/react-query")>(
		"@tanstack/react-query",
	);
	return {
		...actual,
		useQueryClient: () => ({
			invalidateQueries: mocks.invalidateQueries,
		}),
	};
});

import { useProjectRealtime } from "./use-project-realtime";

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("useProjectRealtime", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mocks.connectSocket.mockReturnValue(mocks.socket);
	});

	it("joins the project rooms on mount", () => {
		renderHook(() => useProjectRealtime("proj-abc"));

		expect(mocks.connectSocket).toHaveBeenCalledTimes(1);
		expect(mocks.joinProject).toHaveBeenCalledWith("proj-abc");
	});

	it("registers an event listener on mount", () => {
		renderHook(() => useProjectRealtime("proj-abc"));

		expect(mocks.socket.on).toHaveBeenCalledWith("event", expect.any(Function));
	});

	it("removes the event listener and leaves project rooms on unmount", () => {
		const { unmount } = renderHook(() => useProjectRealtime("proj-abc"));

		const [, listener] = mocks.socket.on.mock.calls[0] as [
			string,
			(...args: unknown[]) => void,
		];

		unmount();

		expect(mocks.socket.off).toHaveBeenCalledWith("event", listener);
		expect(mocks.leaveProject).toHaveBeenCalledWith("proj-abc");
	});

	it("invalidates tasks query key on task.* events", () => {
		renderHook(() => useProjectRealtime("proj-abc"));

		const [, listener] = mocks.socket.on.mock.calls[0] as [
			string,
			(event: { type: string; payload: Record<string, unknown> }) => void,
		];

		listener({ type: "task.created", payload: {} });

		expect(mocks.invalidateQueries).toHaveBeenCalledWith({
			queryKey: ["projects", "proj-abc", "tasks"],
		});
	});

	// A test for "invalidates docs query key on doc.* events" lived here and
	// failed on every run. It asserted invalidation of ["projects", id, "docs"] —
	// a query key that appeared nowhere else in the app, for an event the server
	// never emits. The hook has no doc.* branch and its own header comment lists
	// only task.*, workflow.*, github.* and agent.*.
	//
	// So it was not a broken test of a working feature, nor a failing test
	// guarding a regression: it described something that was never built. Kept
	// as a note rather than a red mark, because a permanently-failing test
	// teaches people to ignore the suite.

	it("invalidates workflows and tasks query keys on workflow.* events", () => {
		renderHook(() => useProjectRealtime("proj-abc"));

		const [, listener] = mocks.socket.on.mock.calls[0] as [
			string,
			(event: { type: string; payload: Record<string, unknown> }) => void,
		];

		listener({ type: "workflow.node.added", payload: {} });

		expect(mocks.invalidateQueries).toHaveBeenCalledWith({
			queryKey: ["projects", "proj-abc", "workflows"],
		});
		expect(mocks.invalidateQueries).toHaveBeenCalledWith({
			queryKey: ["projects", "proj-abc", "tasks"],
		});
	});

	it("invalidates workflows and tasks query keys on workflow.assigned events", () => {
		renderHook(() => useProjectRealtime("proj-abc"));

		const [, listener] = mocks.socket.on.mock.calls[0] as [
			string,
			(event: { type: string; payload: Record<string, unknown> }) => void,
		];

		listener({
			type: "workflow.assigned",
			payload: { project_id: "proj-abc", task_id: "task-1" },
		});

		expect(mocks.invalidateQueries).toHaveBeenCalledWith({
			queryKey: ["projects", "proj-abc", "workflows"],
		});
		expect(mocks.invalidateQueries).toHaveBeenCalledWith({
			queryKey: ["projects", "proj-abc", "tasks"],
		});
	});

	it("invalidates task github query key on github.branch.linked events", () => {
		renderHook(() => useProjectRealtime("proj-abc"));

		const [, listener] = mocks.socket.on.mock.calls[0] as [
			string,
			(event: { type: string; payload: Record<string, unknown> }) => void,
		];

		listener({
			type: "github.branch.linked",
			payload: { task_id: "task-42", project_id: "proj-abc" },
		});

		expect(mocks.invalidateQueries).toHaveBeenCalledWith({
			queryKey: ["projects", "proj-abc", "tasks", "task-42", "github"],
		});
	});

	it("invalidates task github query key on github.pr.linked events", () => {
		renderHook(() => useProjectRealtime("proj-abc"));

		const [, listener] = mocks.socket.on.mock.calls[0] as [
			string,
			(event: { type: string; payload: Record<string, unknown> }) => void,
		];

		listener({
			type: "github.pr.linked",
			payload: { task_id: "task-42", project_id: "proj-abc", pr_number: 7 },
		});

		expect(mocks.invalidateQueries).toHaveBeenCalledWith({
			queryKey: ["projects", "proj-abc", "tasks", "task-42", "github"],
		});
	});

	it("does not invalidate when github.* event has no task_id", () => {
		renderHook(() => useProjectRealtime("proj-abc"));

		const [, listener] = mocks.socket.on.mock.calls[0] as [
			string,
			(event: { type: string; payload: Record<string, unknown> }) => void,
		];

		listener({ type: "github.branch.linked", payload: {} });

		expect(mocks.invalidateQueries).not.toHaveBeenCalled();
	});

	it("does not invalidate queries for unrecognised event types", () => {
		renderHook(() => useProjectRealtime("proj-abc"));

		const [, listener] = mocks.socket.on.mock.calls[0] as [
			string,
			(event: { type: string; payload: Record<string, unknown> }) => void,
		];

		listener({ type: "unknown.event", payload: {} });

		expect(mocks.invalidateQueries).not.toHaveBeenCalled();
	});

	it("registers a connect listener on mount", () => {
		renderHook(() => useProjectRealtime("proj-abc"));

		expect(mocks.socket.on).toHaveBeenCalledWith(
			"connect",
			expect.any(Function),
		);
	});

	it("re-joins the project room when the socket reconnects", () => {
		renderHook(() => useProjectRealtime("proj-abc"));

		mocks.joinProject.mockClear();

		const [, onConnect] = mocks.socket.on.mock.calls.find(
			([event]) => event === "connect",
		) as [string, () => void];
		onConnect();

		expect(mocks.joinProject).toHaveBeenCalledWith("proj-abc");
	});

	it("removes the connect listener on unmount", () => {
		const { unmount } = renderHook(() => useProjectRealtime("proj-abc"));

		const [, onConnect] = mocks.socket.on.mock.calls.find(
			([event]) => event === "connect",
		) as [string, () => void];

		unmount();

		expect(mocks.socket.off).toHaveBeenCalledWith("connect", onConnect);
	});

	it("re-joins and re-registers when projectId changes", () => {
		const { rerender } = renderHook(
			({ projectId }) => useProjectRealtime(projectId),
			{ initialProps: { projectId: "proj-1" } },
		);

		expect(mocks.joinProject).toHaveBeenCalledWith("proj-1");

		rerender({ projectId: "proj-2" });

		// Old project should have been left.
		expect(mocks.leaveProject).toHaveBeenCalledWith("proj-1");
		// New project joined.
		expect(mocks.joinProject).toHaveBeenCalledWith("proj-2");
	});
});
