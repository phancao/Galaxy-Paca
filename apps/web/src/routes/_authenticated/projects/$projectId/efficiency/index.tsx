import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Card } from "@/components/ui/card";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@/components/ui/table";
import { allTasksQueryOptions } from "@/lib/interaction-api";
import {
	projectMembersQueryOptions,
	projectWorklogsQueryOptions,
	type Worklog,
} from "@/lib/project-api";

export const Route = createFileRoute(
	"/_authenticated/projects/$projectId/efficiency/",
)({
	loader: async ({ context: { queryClient }, params: { projectId } }) => {
		await Promise.all([
			queryClient.ensureQueryData(projectMembersQueryOptions(projectId)),
			queryClient.ensureQueryData(projectWorklogsQueryOptions(projectId)),
		]);
	},
	component: EfficiencyPage,
});

const HOURS_PER_DAY = 8;

/** Inclusive business-day count (Mon–Fri) between two dates. */
function businessDays(from: Date, to: Date): number {
	if (to < from) return 0;
	let count = 0;
	const d = new Date(from.getFullYear(), from.getMonth(), from.getDate());
	const end = new Date(to.getFullYear(), to.getMonth(), to.getDate());
	while (d <= end) {
		const dow = d.getDay();
		if (dow !== 0 && dow !== 6) count++;
		d.setDate(d.getDate() + 1);
	}
	return count;
}

interface MemberRow {
	id: string;
	name: string;
	isService: boolean;
	tasks: number;
	estimatedH: number;
	loggedH: number;
	activeDays: number;
	capacityH: number;
	utilization: number; // logged / capacity
}

function EfficiencyPage() {
	const { t } = useTranslation("projects");
	const { projectId } = Route.useParams();
	const { data: members = [] } = useQuery(
		projectMembersQueryOptions(projectId),
	);
	const { data: worklogs = [] } = useQuery(
		projectWorklogsQueryOptions(projectId),
	);
	const { data: taskResult } = useQuery(allTasksQueryOptions(projectId));
	const tasks = taskResult?.items ?? [];

	const rows = useMemo<MemberRow[]>(() => {
		// worklogs by member
		const wlByMember = new Map<string, Worklog[]>();
		for (const wl of worklogs) {
			if (!wl.member_id) continue;
			const arr = wlByMember.get(wl.member_id) ?? [];
			arr.push(wl);
			wlByMember.set(wl.member_id, arr);
		}
		// assigned estimate + task count by member
		const estByMember = new Map<string, number>();
		const taskCountByMember = new Map<string, number>();
		for (const task of tasks) {
			for (const mid of task.assignee_ids ?? []) {
				estByMember.set(
					mid,
					(estByMember.get(mid) ?? 0) + (task.estimate_minutes ?? 0),
				);
				taskCountByMember.set(mid, (taskCountByMember.get(mid) ?? 0) + 1);
			}
		}

		return members
			.map((m): MemberRow => {
				const mwl = wlByMember.get(m.id) ?? [];
				const loggedMin = mwl.reduce((a, w) => a + w.minutes, 0);
				const days = new Set(mwl.map((w) => w.logged_at.slice(0, 10)));
				// Capacity: business days across the member's active window
				// (first→last worklog) × 8h. No worklogs → no capacity.
				let capacityH = 0;
				if (mwl.length > 0) {
					const dates = mwl
						.map((w) => new Date(w.logged_at))
						.sort((a, b) => a.getTime() - b.getTime());
					capacityH =
						businessDays(dates[0], dates[dates.length - 1]) * HOURS_PER_DAY;
				}
				const loggedH = loggedMin / 60;
				return {
					id: m.id,
					name: m.full_name || m.username,
					isService: Boolean(m.is_service),
					tasks: taskCountByMember.get(m.id) ?? 0,
					estimatedH: (estByMember.get(m.id) ?? 0) / 60,
					loggedH,
					activeDays: days.size,
					capacityH,
					utilization: capacityH > 0 ? loggedH / capacityH : 0,
				};
			})
			.filter((r) => r.loggedH > 0 || r.tasks > 0)
			.sort((a, b) => b.loggedH - a.loggedH);
	}, [members, worklogs, tasks]);

	const totals = useMemo(() => {
		const loggedH = rows.reduce((a, r) => a + r.loggedH, 0);
		const estimatedH = rows.reduce((a, r) => a + r.estimatedH, 0);
		const active = rows.filter((r) => r.loggedH > 0).length;
		const avgUtil =
			active > 0
				? rows
						.filter((r) => r.capacityH > 0)
						.reduce((a, r) => a + r.utilization, 0) /
					Math.max(1, rows.filter((r) => r.capacityH > 0).length)
				: 0;
		return { loggedH, estimatedH, active, avgUtil };
	}, [rows]);

	const fmtH = (h: number) => `${h.toFixed(1)}h`;
	const fmtPct = (v: number) => `${Math.round(v * 100)}%`;

	return (
		<div className="p-6 space-y-6">
			<div>
				<h1 className="text-2xl font-bold flex items-center gap-2">
					{t("efficiency.title", { defaultValue: "Team Efficiency" })}
				</h1>
				<p className="text-sm text-muted-foreground">
					{t("efficiency.subtitle", {
						defaultValue:
							"Per-member logged time vs capacity, from tasks + worklogs.",
					})}
				</p>
			</div>

			<div className="grid grid-cols-2 md:grid-cols-4 gap-4">
				<SummaryCard
					label={t("efficiency.membersActive", {
						defaultValue: "Members active",
					})}
					value={String(totals.active)}
				/>
				<SummaryCard
					label={t("efficiency.totalLogged", { defaultValue: "Total logged" })}
					value={fmtH(totals.loggedH)}
				/>
				<SummaryCard
					label={t("efficiency.totalEstimated", {
						defaultValue: "Total estimated",
					})}
					value={fmtH(totals.estimatedH)}
				/>
				<SummaryCard
					label={t("efficiency.avgUtilization", {
						defaultValue: "Avg utilization",
					})}
					value={fmtPct(totals.avgUtil)}
				/>
			</div>

			<Card className="overflow-hidden">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>
								{t("efficiency.member", { defaultValue: "Member" })}
							</TableHead>
							<TableHead className="text-right">
								{t("efficiency.tasks", { defaultValue: "Tasks" })}
							</TableHead>
							<TableHead className="text-right">
								{t("efficiency.estimated", { defaultValue: "Estimated" })}
							</TableHead>
							<TableHead className="text-right">
								{t("efficiency.logged", { defaultValue: "Logged" })}
							</TableHead>
							<TableHead className="text-right">
								{t("efficiency.activeDays", { defaultValue: "Active days" })}
							</TableHead>
							<TableHead className="text-right">
								{t("efficiency.capacity", { defaultValue: "Capacity" })}
							</TableHead>
							<TableHead className="text-right">
								{t("efficiency.utilization", { defaultValue: "Utilization" })}
							</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{rows.length === 0 ? (
							<TableRow>
								<TableCell
									colSpan={7}
									className="text-center text-muted-foreground py-8"
								>
									{t("efficiency.empty", {
										defaultValue: "No logged time yet.",
									})}
								</TableCell>
							</TableRow>
						) : (
							rows.map((r) => (
								<TableRow key={r.id}>
									<TableCell className="font-medium">{r.name}</TableCell>
									<TableCell className="text-right">{r.tasks}</TableCell>
									<TableCell className="text-right">
										{fmtH(r.estimatedH)}
									</TableCell>
									<TableCell className="text-right font-semibold">
										{fmtH(r.loggedH)}
									</TableCell>
									<TableCell className="text-right">{r.activeDays}</TableCell>
									<TableCell className="text-right text-muted-foreground">
										{r.capacityH > 0 ? fmtH(r.capacityH) : "—"}
									</TableCell>
									<TableCell className="text-right">
										<UtilizationBadge
											value={r.utilization}
											hasCapacity={r.capacityH > 0}
											fmt={fmtPct}
										/>
									</TableCell>
								</TableRow>
							))
						)}
					</TableBody>
				</Table>
			</Card>
		</div>
	);
}

function SummaryCard({ label, value }: { label: string; value: string }) {
	return (
		<Card className="p-4">
			<div className="text-xs uppercase tracking-wide text-muted-foreground">
				{label}
			</div>
			<div className="text-2xl font-bold mt-1">{value}</div>
		</Card>
	);
}

function UtilizationBadge({
	value,
	hasCapacity,
	fmt,
}: {
	value: number;
	hasCapacity: boolean;
	fmt: (v: number) => string;
}) {
	if (!hasCapacity) return <span className="text-muted-foreground">—</span>;
	const color =
		value >= 0.85
			? "text-emerald-600"
			: value >= 0.5
				? "text-amber-600"
				: "text-red-600";
	return <span className={`font-semibold ${color}`}>{fmt(value)}</span>;
}
