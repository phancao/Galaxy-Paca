import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Clock, Loader2, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
	createWorklog,
	deleteWorklog,
	type Worklog,
	worklogsQueryOptions,
} from "@/lib/interaction-api";
import { formatDate } from "@/lib/format-date";
import type { ProjectMember } from "@/lib/project-api";
import { formatDuration } from "./helpers";
import { SectionHeading } from "./primitives";

export function TimeTrackingSection({
	projectId,
	taskId,
	estimateMinutes,
	members,
	canEdit = true,
}: {
	projectId: string;
	taskId: string;
	estimateMinutes: number | null | undefined;
	members: ProjectMember[];
	canEdit?: boolean;
}) {
	const { t } = useTranslation("projects");
	const queryClient = useQueryClient();

	const { data: worklogs } = useQuery(worklogsQueryOptions(projectId, taskId));
	const entries = worklogs?.items ?? [];
	const totalMinutes = worklogs?.total_minutes ?? 0;

	const [minutes, setMinutes] = useState("");
	const [note, setNote] = useState("");
	const [error, setError] = useState<string | null>(null);

	const invalidate = () =>
		queryClient.invalidateQueries({
			queryKey: worklogsQueryOptions(projectId, taskId).queryKey,
		});

	const createMutation = useMutation({
		mutationFn: () => {
			const parsed = Number(minutes.trim());
			if (!Number.isFinite(parsed) || parsed <= 0) {
				return Promise.reject(new Error("invalid minutes"));
			}
			return createWorklog(projectId, taskId, {
				minutes: Math.round(parsed),
				note: note.trim() || undefined,
			});
		},
		onSuccess: () => {
			void invalidate();
			setMinutes("");
			setNote("");
			setError(null);
		},
		onError: () => setError(t("taskDetail.timeTracking.errors.logFailed")),
	});

	const deleteMutation = useMutation({
		mutationFn: (worklogId: string) =>
			deleteWorklog(projectId, taskId, worklogId),
		onSuccess: () => void invalidate(),
		onError: () => setError(t("taskDetail.timeTracking.errors.deleteFailed")),
	});

	const memberName = (w: Worklog) => {
		const m = members.find(
			(mm) => mm.id === w.member_id || mm.user_id === w.member_id,
		);
		return m?.full_name || m?.username || t("taskDetail.timeTracking.unknown");
	};

	const parsedMinutes = Number(minutes.trim());
	const canLog =
		Number.isFinite(parsedMinutes) &&
		parsedMinutes > 0 &&
		!createMutation.isPending;

	const estimate = estimateMinutes ?? 0;
	const pct =
		estimate > 0 ? Math.min(100, Math.round((totalMinutes / estimate) * 100)) : 0;

	return (
		<div>
			<SectionHeading>{t("taskDetail.timeTracking.title")}</SectionHeading>

			{/* Summary: logged vs estimate */}
			<div className="rounded-xl border border-border/30 bg-card/50 px-4 py-3.5 space-y-2.5">
				<div className="flex items-center justify-between text-sm">
					<span className="inline-flex items-center gap-1.5 font-medium">
						<Clock className="size-3.5 text-muted-foreground/70" />
						{t("taskDetail.timeTracking.logged", {
							logged: formatDuration(totalMinutes),
						})}
					</span>
					<span className="text-muted-foreground">
						{estimate > 0
							? t("taskDetail.timeTracking.ofEstimate", {
									estimate: formatDuration(estimate),
								})
							: t("taskDetail.timeTracking.noEstimate")}
					</span>
				</div>
				{estimate > 0 && (
					<div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
						<div
							className="h-full rounded-full bg-primary transition-all duration-300"
							style={{ width: `${pct}%` }}
						/>
					</div>
				)}
			</div>

			{/* Log work form */}
			{canEdit && (
				<div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center">
					<Input
						type="number"
						min="1"
						value={minutes}
						onChange={(e) => setMinutes(e.target.value)}
						placeholder={t("taskDetail.timeTracking.minutesPlaceholder")}
						className="h-9 sm:w-32 [appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none"
					/>
					<Input
						value={note}
						onChange={(e) => setNote(e.target.value)}
						onKeyDown={(e) => {
							if (e.key === "Enter" && canLog) createMutation.mutate();
						}}
						placeholder={t("taskDetail.timeTracking.notePlaceholder")}
						className="h-9 flex-1"
					/>
					<Button
						size="sm"
						disabled={!canLog}
						onClick={() => createMutation.mutate()}
						className="shrink-0"
					>
						{createMutation.isPending ? (
							<Loader2 className="size-3.5 animate-spin" />
						) : (
							<Plus className="size-3.5" />
						)}
						{t("taskDetail.timeTracking.logWork")}
					</Button>
				</div>
			)}

			{error ? (
				<p className="mt-2 text-xs text-destructive bg-destructive/10 rounded-lg px-3 py-2">
					{error}
				</p>
			) : null}

			{/* Entries */}
			{entries.length > 0 ? (
				<ul className="mt-3 space-y-1.5">
					{entries.map((w) => (
						<li
							key={w.id}
							className="group flex items-start gap-3 rounded-lg border border-border/30 bg-card/40 px-3.5 py-2.5"
						>
							<div className="min-w-0 flex-1">
								<div className="flex items-center gap-2 text-sm">
									<span className="font-medium">{memberName(w)}</span>
									<span className="rounded-md bg-muted px-1.5 py-0.5 text-xs font-semibold tabular-nums text-muted-foreground">
										{formatDuration(w.minutes)}
									</span>
									<span className="text-xs text-muted-foreground/70">
										{formatDate(w.logged_at)}
									</span>
								</div>
								{w.note ? (
									<p className="mt-0.5 text-xs text-muted-foreground break-words">
										{w.note}
									</p>
								) : null}
							</div>
							{canEdit && (
								<Button
									variant="ghost"
									size="icon-sm"
									className="shrink-0 text-destructive hover:text-destructive hover:bg-destructive/10 opacity-100 transition-opacity sm:opacity-0 sm:group-hover:opacity-100"
									disabled={deleteMutation.isPending}
									onClick={() => deleteMutation.mutate(w.id)}
									title={t("taskDetail.timeTracking.deleteEntry")}
									aria-label={t("taskDetail.timeTracking.deleteEntry")}
								>
									<Trash2 className="size-3.5" />
								</Button>
							)}
						</li>
					))}
				</ul>
			) : (
				<p className="mt-3 text-xs text-muted-foreground/60">
					{t("taskDetail.timeTracking.empty")}
				</p>
			)}
		</div>
	);
}
