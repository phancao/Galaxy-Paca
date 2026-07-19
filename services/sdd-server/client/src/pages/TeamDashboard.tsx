/**
 * @file TeamDashboard.tsx — fleet overview for the coordination server. The
 * lead's landing page: machines online, active devs, tasks by status, open
 * conflicts, pending gates, and recent team activity. Team-wide replacement for
 * the per-machine Dashboard.
 */
import { useCallback, useEffect, useState } from "react";
import {
  Server,
  Users,
  FolderOpen,
  Activity,
  ShieldAlert,
  Lock,
  ListChecks,
  RefreshCw,
} from "lucide-react";
import { api } from "../lib/api";
import { eventBus } from "../lib/eventBus";
import type { TeamOverview } from "../lib/types";

const TASK_COLS = ["todo", "assigned", "in_progress", "review", "done"];

export function TeamDashboard() {
  const [d, setD] = useState<TeamOverview | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const load = useCallback(async () => {
    try {
      setErr(null);
      setD(await api.team.overview());
    } catch (e) {
      setErr(e instanceof Error ? e.message : "load failed");
    }
  }, []);
  useEffect(() => {
    load();
  }, [load]);
  useEffect(() => {
    let t: ReturnType<typeof setTimeout>;
    const u = eventBus.subscribe(() => {
      clearTimeout(t);
      t = setTimeout(load, 2000);
    });
    return () => {
      u();
      clearTimeout(t);
    };
  }, [load]);

  const tile = (
    icon: React.ReactNode,
    label: string,
    value: number | string,
    tone = "text-slate-100"
  ) => (
    <div className="card p-4">
      <div className="flex items-center gap-1.5 text-xs text-slate-400">
        {icon}
        {label}
      </div>
      <div className={`mt-1 text-2xl font-semibold ${tone}`}>{value}</div>
    </div>
  );

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-slate-100">Team overview</h1>
          <p className="mt-1 text-sm text-slate-400">
            Toàn cảnh điều phối cả đội — máy, người, task, xung đột, cổng duyệt
          </p>
        </div>
        <button
          onClick={load}
          className="inline-flex items-center gap-1 rounded-lg border border-slate-600/40 bg-surface-2 px-3 py-1.5 text-sm text-slate-200 hover:bg-slate-700/40"
        >
          <RefreshCw className="h-4 w-4" />
          Refresh
        </button>
      </div>
      {err && (
        <div className="card border-rose-500/30 bg-rose-500/5 p-3 text-sm text-rose-300">{err}</div>
      )}
      {d && (
        <>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
            {tile(
              <Server className="h-4 w-4" />,
              "Máy online",
              d.machines_online,
              "text-emerald-300"
            )}
            {tile(<Users className="h-4 w-4" />, "Dev active", d.active_devs)}
            {tile(<FolderOpen className="h-4 w-4" />, "Phiên active", d.active_sessions)}
            {tile(<Activity className="h-4 w-4" />, "Tổng event", d.total_events)}
            {tile(
              <ShieldAlert className="h-4 w-4" />,
              "Xung đột mở",
              d.open_conflicts,
              d.open_conflicts > 0 ? "text-rose-300" : "text-slate-100"
            )}
            {tile(
              <Lock className="h-4 w-4" />,
              "Gate chờ",
              d.pending_gates,
              d.pending_gates > 0 ? "text-amber-300" : "text-slate-100"
            )}
          </div>

          <div className="card p-4">
            <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold text-slate-200">
              <ListChecks className="h-4 w-4 text-indigo-300" />
              Task theo trạng thái
            </h2>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-5">
              {TASK_COLS.map((s) => (
                <div
                  key={s}
                  className="rounded-lg border border-slate-700/40 bg-surface-2 px-3 py-2"
                >
                  <div className="text-lg font-semibold text-slate-100">
                    {d.tasksByStatus[s] || 0}
                  </div>
                  <div className="text-xs capitalize text-slate-400">{s.replace("_", " ")}</div>
                </div>
              ))}
            </div>
          </div>

          <div className="card p-4">
            <h2 className="mb-3 text-sm font-semibold text-slate-200">Hoạt động đội gần đây</h2>
            <div className="space-y-1">
              {d.recent.length === 0 && (
                <div className="text-sm italic text-slate-500">Chưa có hoạt động</div>
              )}
              {d.recent.map((a, i) => (
                <div
                  key={i}
                  className="flex items-center gap-2 rounded-md border border-slate-700/30 bg-surface-2 px-2 py-1.5 text-xs"
                >
                  {a.level != null && (
                    <span className="shrink-0 rounded border border-slate-600/40 px-1 text-[10px] text-slate-300">
                      L{a.level}
                    </span>
                  )}
                  {a.phase && (
                    <span className="shrink-0 rounded bg-slate-700/40 px-1.5 text-[10px] text-slate-300">
                      {a.phase}
                    </span>
                  )}
                  <span className="truncate text-slate-300">{a.tool_name || "—"}</span>
                  <span className="ml-auto shrink-0 text-[10px] text-slate-500">
                    {a.user_name} @ {a.hostname || "?"}
                  </span>
                </div>
              ))}
            </div>
          </div>
        </>
      )}
    </div>
  );
}
