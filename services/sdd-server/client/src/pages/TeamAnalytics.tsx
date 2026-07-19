/**
 * @file TeamAnalytics.tsx — team-wide analytics for the coordination server:
 * activity by developer / machine / repo, phase + governance-level distribution,
 * and a 14-day activity trend. Team replacement for the per-machine Analytics.
 */
import { useCallback, useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
import { api } from "../lib/api";
import type { TeamAnalytics as TA, TeamBar } from "../lib/types";

function Bars({
  title,
  data,
  color = "bg-indigo-500/60",
}: {
  title: string;
  data: TeamBar[];
  color?: string;
}) {
  const max = Math.max(1, ...data.map((d) => d.n));
  return (
    <div className="card p-4">
      <h2 className="mb-3 text-sm font-semibold text-slate-200">{title}</h2>
      {data.length === 0 ? (
        <div className="text-sm italic text-slate-500">—</div>
      ) : (
        <div className="space-y-1.5">
          {data.map((d, i) => (
            <div key={i} className="flex items-center gap-2 text-xs">
              <span className="w-32 shrink-0 truncate text-slate-300" title={d.label}>
                {d.label}
              </span>
              <div className="h-3 flex-1 overflow-hidden rounded bg-surface-2">
                <div className={`h-full ${color}`} style={{ width: `${(d.n / max) * 100}%` }} />
              </div>
              <span className="w-8 shrink-0 text-right text-slate-400">{d.n}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

const LVL = ["", "L1 Read", "L2 Branch", "L3 Shared Core", "L4 Merge/Deploy"];
const LVLC = ["", "bg-slate-500/50", "bg-sky-500/50", "bg-amber-500/50", "bg-rose-500/50"];

export function TeamAnalytics() {
  const [d, setD] = useState<TA | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const load = useCallback(async () => {
    try {
      setErr(null);
      setD(await api.team.analytics());
    } catch (e) {
      setErr(e instanceof Error ? e.message : "load failed");
    }
  }, []);
  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-slate-100">Team analytics</h1>
          <p className="mt-1 text-sm text-slate-400">
            Hoạt động cả đội theo người, máy, repo, giai đoạn, mức phân quyền
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
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
            <Bars title="Theo dev" data={d.byUser} />
            <Bars title="Theo máy" data={d.byHost} color="bg-emerald-500/50" />
            <Bars title="Theo repo" data={d.byRepo} color="bg-sky-500/50" />
          </div>
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Bars title="Phân bố giai đoạn" data={d.phaseDist} color="bg-violet-500/50" />
            <div className="card p-4">
              <h2 className="mb-3 text-sm font-semibold text-slate-200">Phân bố mức phân quyền</h2>
              <div className="space-y-1.5">
                {d.levelDist.length === 0 ? (
                  <div className="text-sm italic text-slate-500">—</div>
                ) : (
                  d.levelDist.map((l) => {
                    const max = Math.max(1, ...d.levelDist.map((x) => x.n));
                    return (
                      <div key={l.level} className="flex items-center gap-2 text-xs">
                        <span className="w-32 shrink-0 text-slate-300">
                          {LVL[l.level] || `L${l.level}`}
                        </span>
                        <div className="h-3 flex-1 overflow-hidden rounded bg-surface-2">
                          <div
                            className={`h-full ${LVLC[l.level] || "bg-slate-500/50"}`}
                            style={{ width: `${(l.n / max) * 100}%` }}
                          />
                        </div>
                        <span className="w-8 shrink-0 text-right text-slate-400">{l.n}</span>
                      </div>
                    );
                  })
                )}
              </div>
            </div>
          </div>
          <div className="card p-4">
            <h2 className="mb-3 text-sm font-semibold text-slate-200">Hoạt động 14 ngày</h2>
            <div className="flex items-end gap-1" style={{ height: 80 }}>
              {d.daily.length === 0 ? (
                <div className="text-sm italic text-slate-500">—</div>
              ) : (
                d.daily.map((x) => {
                  const max = Math.max(1, ...d.daily.map((y) => y.n));
                  return (
                    <div
                      key={x.day}
                      className="flex flex-1 flex-col items-center justify-end gap-1"
                      title={`${x.day}: ${x.n}`}
                    >
                      <div
                        className="w-full rounded-t bg-indigo-500/50"
                        style={{ height: `${(x.n / max) * 64}px` }}
                      />
                      <span className="text-[9px] text-slate-500">{x.day}</span>
                    </div>
                  );
                })
              )}
            </div>
          </div>
        </>
      )}
    </div>
  );
}
