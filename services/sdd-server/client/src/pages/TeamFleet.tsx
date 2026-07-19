/**
 * @file TeamFleet.tsx — the fleet of developer machines reporting in: who, which
 * host, how many sessions, last seen, and their current SDD phase/level. Team
 * replacement for the per-machine Claude Config tab.
 */
import { useCallback, useEffect, useState } from "react";
import { RefreshCw, Server, Circle } from "lucide-react";
import { api } from "../lib/api";
import type { FleetMachine } from "../lib/types";

function ago(iso: string | null): string {
  if (!iso) return "—";
  const s = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  if (s < 86400) return `${Math.floor(s / 3600)}h`;
  return `${Math.floor(s / 86400)}d`;
}
const online = (iso: string | null) =>
  !!iso && Date.now() - new Date(iso).getTime() < 15 * 60 * 1000;

export function TeamFleet() {
  const [machines, setMachines] = useState<FleetMachine[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const load = useCallback(async () => {
    try {
      setErr(null);
      setMachines((await api.team.fleet()).machines);
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
          <h1 className="text-xl font-semibold text-slate-100">Fleet máy</h1>
          <p className="mt-1 text-sm text-slate-400">
            Các máy/dev đang báo về server — trạng thái, phiên, giai đoạn hiện tại
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
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {machines.length === 0 && (
          <div className="text-sm italic text-slate-500">Chưa có máy nào báo về</div>
        )}
        {machines.map((m, i) => (
          <div key={i} className="card p-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-1.5">
                <Server className="h-4 w-4 text-slate-400" />
                <span className="text-sm font-medium text-slate-100">{m.hostname || "?"}</span>
              </div>
              <span className="inline-flex items-center gap-1 text-[10px]">
                <Circle
                  className={`h-2 w-2 ${online(m.last_seen) ? "fill-emerald-400 text-emerald-400" : "fill-slate-600 text-slate-600"}`}
                />
                {online(m.last_seen) ? "online" : ago(m.last_seen)}
              </span>
            </div>
            <div className="mt-1 text-xs text-slate-400">{m.user_name}</div>
            <div className="mt-2 flex items-center gap-2 text-[11px] text-slate-400">
              <span>{m.sessions} phiên</span>
              {m.current_phase && (
                <span className="rounded bg-slate-700/40 px-1.5 text-[10px] text-slate-300">
                  {m.current_phase}
                </span>
              )}
              {m.current_level != null && (
                <span className="text-[10px] text-slate-300">L{m.current_level}</span>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
