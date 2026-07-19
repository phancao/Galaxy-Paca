/**
 * @file Sdd.tsx
 * @description Spec-Driven Development monitor tab. Surfaces the Galaxy Agentic
 * SDLC dimensions the server classifies: an 8-phase board of live agents, the
 * governance-level mix (L1 read .. L4 prod/merge), tracked spec versions, the
 * Shared Core / unapproved-L3 governance flags, and recent SDD activity. Reads
 * /api/sdd/* and refreshes on the `sdd_updated` WebSocket event.
 */

import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  ListChecks,
  RefreshCw,
  AlertCircle,
  ShieldAlert,
  GitBranch,
  FileStack,
  CheckCircle2,
} from "lucide-react";
import { api } from "../lib/api";
import { eventBus } from "../lib/eventBus";
import type {
  SddOverview,
  SddSpecVersionsResult,
  SddFlagsResult,
  SddActivity,
  WSMessage,
} from "../lib/types";

const LEVELS = [
  { n: 1, key: "l1", color: "text-slate-300", bg: "bg-slate-500/15 border-slate-500/25" },
  { n: 2, key: "l2", color: "text-sky-300", bg: "bg-sky-500/15 border-sky-500/25" },
  { n: 3, key: "l3", color: "text-amber-300", bg: "bg-amber-500/15 border-amber-500/30" },
  { n: 4, key: "l4", color: "text-rose-300", bg: "bg-rose-500/15 border-rose-500/30" },
] as const;

function levelBadge(level: number | null) {
  const l = LEVELS.find((x) => x.n === level);
  if (!l) return "bg-slate-500/10 text-slate-400 border-slate-500/20";
  return `${l.bg} ${l.color}`;
}

export function Sdd() {
  const { t } = useTranslation("sdd");
  const [overview, setOverview] = useState<SddOverview | null>(null);
  const [specs, setSpecs] = useState<SddSpecVersionsResult | null>(null);
  const [flags, setFlags] = useState<SddFlagsResult | null>(null);
  const [syncConfigured, setSyncConfigured] = useState<boolean>(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);

  const fetchData = useCallback(async () => {
    try {
      setError(null);
      const [ov, sp, fl, ss] = await Promise.all([
        api.sdd.overview(),
        api.sdd.specVersions(),
        api.sdd.flags(),
        api.sdd.specSyncStatus().catch(() => ({ configured: false, hasToken: false })),
      ]);
      setOverview(ov);
      setSpecs(sp);
      setFlags(fl);
      setSyncConfigured(!!ss.configured);
      setLastUpdated(new Date());
    } catch (err) {
      setError(err instanceof Error ? err.message : t("failedLoad"));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  // Refresh on SDD websocket events (debounced).
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout>;
    const handler = (msg: WSMessage) => {
      if (msg.type !== "sdd_updated") return;
      clearTimeout(timer);
      timer = setTimeout(fetchData, 1500);
    };
    const unsub = eventBus.subscribe(handler);
    return () => {
      unsub();
      clearTimeout(timer);
    };
  }, [fetchData]);

  const header = (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 className="flex items-center gap-2 text-xl font-semibold text-slate-100">
          <ListChecks className="h-5 w-5 text-emerald-400" />
          {t("title")}
        </h1>
        <p className="mt-1 text-sm text-slate-400">{t("subtitle")}</p>
      </div>
      <div className="flex items-center gap-2">
        <span
          className={`inline-flex items-center gap-1 rounded-full border px-2 py-1 text-xs ${
            syncConfigured
              ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-300"
              : "border-slate-600/40 bg-slate-700/20 text-slate-400"
          }`}
          title={syncConfigured ? t("specSyncOn") : t("specSyncOff")}
        >
          <GitBranch className="h-3 w-3" />
          {syncConfigured ? t("specSyncOn") : t("specSyncOff")}
        </span>
        <button
          onClick={() => {
            setLoading(true);
            fetchData();
          }}
          className="inline-flex items-center gap-1 rounded-lg border border-slate-600/40 bg-surface-2 px-3 py-1.5 text-sm text-slate-200 hover:bg-slate-700/40"
        >
          <RefreshCw className="h-4 w-4" />
          {t("refresh")}
        </button>
      </div>
    </div>
  );

  if (loading && !overview) {
    return (
      <div className="space-y-6">
        {header}
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="card h-20 animate-pulse bg-surface-2" />
          ))}
        </div>
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          {Array.from({ length: 8 }).map((_, i) => (
            <div key={i} className="card h-40 animate-pulse bg-surface-2" />
          ))}
        </div>
      </div>
    );
  }

  if (error && !overview) {
    return (
      <div className="space-y-6">
        {header}
        <div className="card flex items-center gap-3 border-rose-500/30 bg-rose-500/5 p-4 text-rose-300">
          <AlertCircle className="h-5 w-5" />
          {error}
        </div>
      </div>
    );
  }

  const ov = overview!;
  const totalPhaseAgents = Object.values(ov.board).reduce((a, b) => a + b.length, 0);

  return (
    <div className="space-y-6">
      {header}

      {/* Summary stat row */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatTile
          label={t("phaseBoard")}
          value={totalPhaseAgents}
          icon={<ListChecks className="h-4 w-4" />}
        />
        <StatTile
          label={t("specVersions")}
          value={specs?.count ?? 0}
          icon={<FileStack className="h-4 w-4" />}
        />
        <StatTile
          label={t("sharedCoreTouches")}
          value={ov.sharedCoreCount}
          tone={ov.sharedCoreCount > 0 ? "amber" : "default"}
          icon={<ShieldAlert className="h-4 w-4" />}
        />
        <StatTile
          label={t("unapprovedL3")}
          value={ov.unapprovedL3Count}
          tone={ov.unapprovedL3Count > 0 ? "rose" : "default"}
          icon={<ShieldAlert className="h-4 w-4" />}
        />
      </div>

      {/* Governance level mix */}
      <div className="card p-4">
        <h2 className="mb-3 text-sm font-semibold text-slate-200">{t("governance")}</h2>
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
          {LEVELS.map((l) => (
            <div key={l.n} className={`rounded-lg border px-3 py-2 ${l.bg}`}>
              <div className={`text-lg font-semibold ${l.color}`}>
                {ov.levelCounts?.[String(l.n)] ?? 0}
              </div>
              <div className={`text-xs ${l.color}`}>{t(l.key)}</div>
            </div>
          ))}
        </div>
      </div>

      {/* Phase board */}
      <div>
        <h2 className="mb-3 text-sm font-semibold text-slate-200">{t("phaseBoard")}</h2>
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          {ov.phases.map((p) => {
            const agents = ov.board[p.key] || [];
            return (
              <div key={p.key} className="card flex flex-col p-3">
                <div className="mb-2 flex items-center justify-between">
                  <span className="text-sm font-medium text-slate-100">{p.label}</span>
                  <span className="rounded-full bg-slate-700/40 px-2 py-0.5 text-xs text-slate-300">
                    {agents.length}
                  </span>
                </div>
                {p.owner && <div className="mb-2 text-[11px] text-slate-500">{p.owner}</div>}
                <div className="space-y-1.5">
                  {agents.length === 0 ? (
                    <div className="text-[11px] italic text-slate-600">{t("noAgents")}</div>
                  ) : (
                    agents.map((a) => (
                      <div
                        key={a.id}
                        className="rounded-md border border-slate-700/40 bg-surface-2 px-2 py-1.5"
                      >
                        <div className="flex items-center justify-between gap-1">
                          <span className="truncate text-xs text-slate-200" title={a.name}>
                            {a.name}
                          </span>
                          {a.sdd_level != null && (
                            <span
                              className={`shrink-0 rounded border px-1 text-[10px] ${levelBadge(a.sdd_level)}`}
                            >
                              L{a.sdd_level}
                            </span>
                          )}
                        </div>
                        {a.spec_version && (
                          <div className="mt-0.5 truncate text-[10px] text-slate-500">
                            {a.spec_doc_id}@{a.spec_version}
                          </div>
                        )}
                      </div>
                    ))
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Flags + spec versions */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <div className="card p-4">
          <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold text-slate-200">
            <ShieldAlert className="h-4 w-4 text-amber-400" />
            {t("flags")}
          </h2>
          {flags && (flags.sharedCore.length > 0 || flags.unapprovedL3.length > 0) ? (
            <div className="space-y-3">
              {flags.unapprovedL3.length > 0 && (
                <div>
                  <div className="mb-1 text-xs font-medium text-rose-300">{t("unapprovedL3")}</div>
                  <ActivityList items={flags.unapprovedL3.slice(0, 8)} levelBadge={levelBadge} />
                </div>
              )}
              {flags.sharedCore.length > 0 && (
                <div>
                  <div className="mb-1 text-xs font-medium text-amber-300">
                    {t("sharedCoreTouches")}
                  </div>
                  <ActivityList items={flags.sharedCore.slice(0, 8)} levelBadge={levelBadge} />
                </div>
              )}
            </div>
          ) : (
            <div className="flex items-center gap-2 text-sm text-emerald-300">
              <CheckCircle2 className="h-4 w-4" />
              {t("noFlags")}
            </div>
          )}
        </div>

        <div className="card p-4">
          <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold text-slate-200">
            <FileStack className="h-4 w-4 text-sky-400" />
            {t("specVersions")}
          </h2>
          {specs && specs.count > 0 ? (
            <div className="space-y-3">
              {Object.entries(specs.docs).map(([docId, versions]) => (
                <div key={docId}>
                  <div className="mb-1 truncate text-xs font-medium text-slate-200" title={docId}>
                    {docId}
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    {versions.map((v) => (
                      <span
                        key={v.id}
                        className="inline-flex items-center gap-1 rounded border border-sky-500/25 bg-sky-500/10 px-1.5 py-0.5 text-[11px] text-sky-200"
                        title={v.title || undefined}
                      >
                        v{v.version}
                        {v.implemented_ref && (
                          <span className="text-emerald-300">· {t("implemented")}</span>
                        )}
                      </span>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="text-sm italic text-slate-500">{t("noSpecs")}</div>
          )}
        </div>
      </div>

      {/* Recent activity */}
      <div className="card p-4">
        <h2 className="mb-3 text-sm font-semibold text-slate-200">{t("recentActivity")}</h2>
        {ov.recent.length === 0 ? (
          <div className="text-sm italic text-slate-500">{t("noActivity")}</div>
        ) : (
          <ActivityList items={ov.recent.slice(0, 25)} levelBadge={levelBadge} showPhase />
        )}
      </div>

      {lastUpdated && (
        <div className="text-right text-xs text-slate-600">
          {t("lastUpdated")} {lastUpdated.toLocaleTimeString()}
        </div>
      )}
    </div>
  );
}

function StatTile({
  label,
  value,
  icon,
  tone = "default",
}: {
  label: string;
  value: number;
  icon?: React.ReactNode;
  tone?: "default" | "amber" | "rose";
}) {
  const toneCls =
    tone === "rose" ? "text-rose-300" : tone === "amber" ? "text-amber-300" : "text-slate-100";
  return (
    <div className="card p-3">
      <div className="flex items-center gap-1.5 text-xs text-slate-400">
        {icon}
        {label}
      </div>
      <div className={`mt-1 text-2xl font-semibold ${toneCls}`}>{value}</div>
    </div>
  );
}

function ActivityList({
  items,
  levelBadge,
  showPhase = false,
}: {
  items: SddActivity[];
  levelBadge: (level: number | null) => string;
  showPhase?: boolean;
}) {
  return (
    <div className="space-y-1">
      {items.map((a) => (
        <div
          key={a.id}
          className="flex items-center gap-2 rounded-md border border-slate-700/30 bg-surface-2 px-2 py-1.5 text-xs"
        >
          {a.level != null && (
            <span className={`shrink-0 rounded border px-1 text-[10px] ${levelBadge(a.level)}`}>
              L{a.level}
            </span>
          )}
          {showPhase && a.phase && (
            <span className="shrink-0 rounded bg-slate-700/40 px-1.5 text-[10px] text-slate-300">
              {a.phase}
            </span>
          )}
          {a.lifecycle && (
            <span className="shrink-0 rounded bg-sky-500/15 px-1.5 text-[10px] text-sky-300">
              {a.lifecycle}
            </span>
          )}
          <span className="truncate text-slate-300" title={a.tool_name || a.summary || ""}>
            {a.tool_name || a.summary || a.hook_type}
          </span>
          {a.file_path && (
            <span className="ml-auto truncate text-[10px] text-slate-600" title={a.file_path}>
              {a.file_path.split("/").slice(-2).join("/")}
            </span>
          )}
        </div>
      ))}
    </div>
  );
}
