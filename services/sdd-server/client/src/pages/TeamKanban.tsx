/**
 * @file TeamKanban.tsx
 * @description The cross-machine TASK coordination board (central server). A
 * lead creates a task and assigns it to a developer; that dev's machine
 * "receives" it; the card tracks live progress by showing the assignee's
 * current SDD phase/level pulled from their live activity. Replaces the
 * per-machine Kanban (agents-by-status) on the coordination server.
 */

import { useCallback, useEffect, useState } from "react";
import { Plus, RefreshCw, AlertCircle, Trash2, User, Server } from "lucide-react";
import { api } from "../lib/api";
import { eventBus } from "../lib/eventBus";
import type { Task, TaskAssignee, WSMessage } from "../lib/types";

const COLUMNS: { key: Task["status"]; label: string }[] = [
  { key: "todo", label: "Todo" },
  { key: "assigned", label: "Assigned" },
  { key: "in_progress", label: "In Progress" },
  { key: "review", label: "Review" },
  { key: "done", label: "Done" },
];
const NEXT: Record<Task["status"], Task["status"] | null> = {
  todo: "assigned",
  assigned: "in_progress",
  in_progress: "review",
  review: "done",
  done: null,
};
const PREV: Record<Task["status"], Task["status"] | null> = {
  todo: null,
  assigned: "todo",
  in_progress: "assigned",
  review: "in_progress",
  done: "review",
};
const PRIORITY: Record<string, string> = {
  high: "border-rose-500/40 text-rose-300",
  normal: "border-slate-600/40 text-slate-300",
  low: "border-slate-700/40 text-slate-500",
};
function levelColor(l: number | null) {
  return l === 4
    ? "text-rose-300"
    : l === 3
      ? "text-amber-300"
      : l === 2
        ? "text-sky-300"
        : "text-slate-400";
}

export function TeamKanban() {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [assignees, setAssignees] = useState<TaskAssignee[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showNew, setShowNew] = useState(false);
  const [form, setForm] = useState({
    title: "",
    assignee_user_id: "",
    repo: "",
    priority: "normal",
  });

  const load = useCallback(async () => {
    try {
      setError(null);
      const [t, a] = await Promise.all([api.tasks.list(), api.tasks.assignees()]);
      setTasks(t.tasks);
      setAssignees(a.assignees);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load tasks");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout>;
    const unsub = eventBus.subscribe((msg: WSMessage) => {
      if (msg.type !== "task_updated" && msg.type !== "sdd_updated") return;
      clearTimeout(timer);
      timer = setTimeout(load, 1200);
    });
    return () => {
      unsub();
      clearTimeout(timer);
    };
  }, [load]);

  async function move(t: Task, status: Task["status"]) {
    setTasks((cur) => cur.map((x) => (x.id === t.id ? { ...x, status } : x)));
    try {
      await api.tasks.update(t.id, { status });
    } catch {
      load();
    }
  }
  async function create() {
    if (!form.title.trim()) return;
    const status = form.assignee_user_id ? "assigned" : "todo";
    await api.tasks.create({
      title: form.title.trim(),
      assignee_user_id: form.assignee_user_id || null,
      repo: form.repo || null,
      priority: form.priority as Task["priority"],
      status,
    });
    setForm({ title: "", assignee_user_id: "", repo: "", priority: "normal" });
    setShowNew(false);
    load();
  }
  async function remove(t: Task) {
    setTasks((cur) => cur.filter((x) => x.id !== t.id));
    try {
      await api.tasks.remove(t.id);
    } catch {
      load();
    }
  }

  const header = (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 className="text-xl font-semibold text-slate-100">Task coordination</h1>
        <p className="mt-1 text-sm text-slate-400">
          Giao task cho máy/dev và theo dõi tiến độ live của cả đội
        </p>
      </div>
      <div className="flex items-center gap-2">
        <button
          onClick={() => setShowNew((v) => !v)}
          className="inline-flex items-center gap-1 rounded-lg bg-indigo-500/80 px-3 py-1.5 text-sm text-white hover:bg-indigo-500"
        >
          <Plus className="h-4 w-4" /> New task
        </button>
        <button
          onClick={load}
          className="inline-flex items-center gap-1 rounded-lg border border-slate-600/40 bg-surface-2 px-3 py-1.5 text-sm text-slate-200 hover:bg-slate-700/40"
        >
          <RefreshCw className="h-4 w-4" /> Refresh
        </button>
      </div>
    </div>
  );

  if (loading && !tasks.length) {
    return (
      <div className="space-y-6">
        {header}
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="card h-64 animate-pulse bg-surface-2" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}

      {showNew && (
        <div className="card space-y-3 p-4">
          <input
            autoFocus
            value={form.title}
            onChange={(e) => setForm({ ...form, title: e.target.value })}
            placeholder="Task title…"
            className="w-full rounded-lg border border-slate-600/40 bg-surface-2 px-3 py-2 text-sm text-slate-100 outline-none focus:border-indigo-500/50"
          />
          <div className="flex flex-wrap gap-2">
            <select
              value={form.assignee_user_id}
              onChange={(e) => setForm({ ...form, assignee_user_id: e.target.value })}
              className="rounded-lg border border-slate-600/40 bg-surface-2 px-2 py-1.5 text-sm text-slate-200"
            >
              <option value="">— Assign to —</option>
              {assignees.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name || a.email || a.id}
                  {a.hosts.length ? ` @ ${a.hosts.join(", ")}` : ""}
                </option>
              ))}
            </select>
            <input
              value={form.repo}
              onChange={(e) => setForm({ ...form, repo: e.target.value })}
              placeholder="repo (optional)"
              className="rounded-lg border border-slate-600/40 bg-surface-2 px-2 py-1.5 text-sm text-slate-200"
            />
            <select
              value={form.priority}
              onChange={(e) => setForm({ ...form, priority: e.target.value })}
              className="rounded-lg border border-slate-600/40 bg-surface-2 px-2 py-1.5 text-sm text-slate-200"
            >
              <option value="low">low</option>
              <option value="normal">normal</option>
              <option value="high">high</option>
            </select>
            <button
              onClick={create}
              className="rounded-lg bg-indigo-500/80 px-3 py-1.5 text-sm text-white hover:bg-indigo-500"
            >
              Create
            </button>
          </div>
        </div>
      )}

      {error && (
        <div className="card flex items-center gap-2 border-rose-500/30 bg-rose-500/5 p-3 text-sm text-rose-300">
          <AlertCircle className="h-4 w-4" /> {error}
        </div>
      )}

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-5">
        {COLUMNS.map((col) => {
          const items = tasks.filter((t) => t.status === col.key);
          return (
            <div key={col.key} className="card flex flex-col p-3">
              <div className="mb-2 flex items-center justify-between">
                <span className="text-sm font-medium text-slate-100">{col.label}</span>
                <span className="rounded-full bg-slate-700/40 px-2 py-0.5 text-xs text-slate-300">
                  {items.length}
                </span>
              </div>
              <div className="space-y-2">
                {items.length === 0 && <div className="text-[11px] italic text-slate-600">—</div>}
                {items.map((t) => (
                  <div
                    key={t.id}
                    className={`rounded-md border bg-surface-2 px-2.5 py-2 ${PRIORITY[t.priority] || PRIORITY.normal}`}
                  >
                    <div className="flex items-start justify-between gap-1">
                      <span className="text-xs font-medium text-slate-100">{t.title}</span>
                      <button
                        onClick={() => remove(t)}
                        title="Delete"
                        className="text-slate-600 hover:text-rose-400"
                      >
                        <Trash2 className="h-3 w-3" />
                      </button>
                    </div>
                    {(t.assignee_name || t.assignee_hostname) && (
                      <div className="mt-1 flex items-center gap-1 text-[10px] text-slate-400">
                        <User className="h-3 w-3" />
                        {t.assignee_name || t.assignee_email || "unassigned"}
                        {t.assignee_hostname && (
                          <>
                            <Server className="h-3 w-3" /> {t.assignee_hostname}
                          </>
                        )}
                      </div>
                    )}
                    {t.repo && <div className="mt-0.5 text-[10px] text-slate-500">{t.repo}</div>}
                    {t.live_phase && (
                      <div className="mt-1 flex items-center gap-1">
                        <span className="rounded bg-slate-700/40 px-1.5 text-[10px] text-slate-300">
                          {t.live_phase}
                        </span>
                        {t.live_level != null && (
                          <span className={`text-[10px] ${levelColor(t.live_level)}`}>
                            L{t.live_level}
                          </span>
                        )}
                        <span className="text-[9px] text-emerald-400">● live</span>
                      </div>
                    )}
                    <div className="mt-1.5 flex items-center justify-between">
                      {PREV[t.status] ? (
                        <button
                          onClick={() => move(t, PREV[t.status]!)}
                          className="rounded px-1 text-[10px] text-slate-400 hover:text-slate-200"
                        >
                          ← {COLUMNS.find((c) => c.key === PREV[t.status])?.label}
                        </button>
                      ) : (
                        <span />
                      )}
                      {NEXT[t.status] && (
                        <button
                          onClick={() => move(t, NEXT[t.status]!)}
                          className="rounded px-1 text-[10px] text-indigo-300 hover:text-indigo-200"
                        >
                          {COLUMNS.find((c) => c.key === NEXT[t.status])?.label} →
                        </button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
