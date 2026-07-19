/**
 * @file TeamCoordination.tsx — cross-machine coordination view: open conflicts
 * (Shared Core parallel edits across machines) and a per-repo map of who is
 * working where in parallel. Team replacement for the per-machine Workflows tab.
 */
import { useCallback, useEffect, useState } from "react";
import { RefreshCw, ShieldAlert, GitBranch, Users, Server } from "lucide-react";
import { api } from "../lib/api";
import { eventBus } from "../lib/eventBus";
import type { TeamCoordination as TC, WSMessage } from "../lib/types";

export function TeamCoordination() {
  const [d, setD] = useState<TC | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const load = useCallback(async () => {
    try {
      setErr(null);
      setD(await api.team.coordination());
    } catch (e) {
      setErr(e instanceof Error ? e.message : "load failed");
    }
  }, []);
  useEffect(() => {
    load();
  }, [load]);
  useEffect(() => {
    let t: ReturnType<typeof setTimeout>;
    const u = eventBus.subscribe((m: WSMessage) => {
      if (m.type === "sdd_conflict" || m.type === "sdd_updated") {
        clearTimeout(t);
        t = setTimeout(load, 1500);
      }
    });
    return () => {
      u();
      clearTimeout(t);
    };
  }, [load]);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-slate-100">Coordination</h1>
          <p className="mt-1 text-sm text-slate-400">
            Điều phối chéo máy — xung đột Shared Core và ai làm song song ở repo nào
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
          <div className="card p-4">
            <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold text-slate-200">
              <ShieldAlert className="h-4 w-4 text-rose-400" />
              Xung đột mở
            </h2>
            {d.conflicts.length === 0 ? (
              <div className="text-sm text-emerald-300">
                Không có xung đột — chưa có ai giẫm chân nhau
              </div>
            ) : (
              <div className="space-y-1.5">
                {d.conflicts.map((c) => (
                  <div
                    key={c.id}
                    className="rounded-md border border-rose-500/30 bg-rose-500/5 px-2.5 py-2 text-xs"
                  >
                    <div className="flex items-center gap-2">
                      <span className="rounded bg-rose-500/20 px-1.5 text-[10px] text-rose-300">
                        {c.kind}
                      </span>
                      <span className="truncate text-slate-300">{c.conflict_key}</span>
                    </div>
                    <div className="mt-1 text-[11px] text-slate-400">
                      {JSON.stringify((c.detail as { users?: string[] })?.users || [])}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className="card p-4">
            <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold text-slate-200">
              <GitBranch className="h-4 w-4 text-sky-400" />
              Làm song song theo repo
            </h2>
            {d.byRepo.length === 0 ? (
              <div className="text-sm italic text-slate-500">—</div>
            ) : (
              <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
                {d.byRepo.map((r) => (
                  <div
                    key={r.repo}
                    className="rounded-lg border border-slate-700/40 bg-surface-2 p-3"
                  >
                    <div className="text-sm font-medium text-slate-100">{r.repo}</div>
                    <div className="mt-1 flex items-center gap-3 text-[11px] text-slate-400">
                      <span className="inline-flex items-center gap-1">
                        <Users className="h-3 w-3" />
                        {r.devs}
                      </span>
                      <span className="inline-flex items-center gap-1">
                        <Server className="h-3 w-3" />
                        {r.machines}
                      </span>
                      <span>{r.sessions} phiên</span>
                    </div>
                    {r.dev_names?.length > 0 && (
                      <div className="mt-1 truncate text-[10px] text-slate-500">
                        {r.dev_names.join(", ")}
                      </div>
                    )}
                    {r.phases?.length > 0 && (
                      <div className="mt-1 flex flex-wrap gap-1">
                        {r.phases.map((p) => (
                          <span
                            key={p}
                            className="rounded bg-slate-700/40 px-1.5 text-[10px] text-slate-300"
                          >
                            {p}
                          </span>
                        ))}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        </>
      )}
    </div>
  );
}
