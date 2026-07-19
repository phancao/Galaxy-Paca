import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, GitBranch, Loader2, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@/components/ui/table";
import {
	createStatusTransition,
	customFieldsQueryOptions,
	deleteStatusTransition,
	type StatusTransition,
	statusTransitionsQueryOptions,
	taskStatusesQueryOptions,
	taskTypesQueryOptions,
} from "@/lib/project-api";
import { cn } from "@/lib/utils";

// Sentinels for the "any source status" / "all task types" scope options.
const ANY_STATUS = "__any__";
const ALL_TYPES = "__all__";

const PILL_BASE =
	"rounded-lg border px-2.5 py-1 text-xs font-medium transition-colors";
const PILL_SELECTED = "border-primary bg-primary/10 text-primary";
const PILL_UNSELECTED =
	"border-border/60 text-muted-foreground hover:border-border hover:bg-muted/50";

function isConflict(err: unknown): boolean {
	return (err as { response?: { status?: number } })?.response?.status === 409;
}

export function WorkflowSettings({
	projectId,
	canWrite,
}: {
	projectId: string;
	canWrite: boolean;
}) {
	const { t } = useTranslation("projects");
	const queryClient = useQueryClient();

	const { data: transitions = [], isLoading } = useQuery(
		statusTransitionsQueryOptions(projectId),
	);
	const { data: statuses = [] } = useQuery(taskStatusesQueryOptions(projectId));
	const { data: taskTypes = [] } = useQuery(taskTypesQueryOptions(projectId));
	const { data: customFields = [] } = useQuery(
		customFieldsQueryOptions(projectId),
	);

	const [fromStatusId, setFromStatusId] = useState<string>(ANY_STATUS);
	const [toStatusId, setToStatusId] = useState<string | null>(null);
	const [taskTypeId, setTaskTypeId] = useState<string>(ALL_TYPES);
	const [requiredFields, setRequiredFields] = useState<string[]>([]);
	const [error, setError] = useState<string | null>(null);

	const sortedStatuses = [...statuses].sort((a, b) => a.position - b.position);

	const statusName = (id: string | null) =>
		id ? (statuses.find((s) => s.id === id)?.name ?? id) : null;
	const typeName = (id: string | null) =>
		id ? (taskTypes.find((tt) => tt.id === id)?.name ?? id) : null;
	const fieldName = (key: string) =>
		customFields.find((cf) => cf.field_key === key)?.display_name ?? key;

	const resetForm = () => {
		setFromStatusId(ANY_STATUS);
		setToStatusId(null);
		setTaskTypeId(ALL_TYPES);
		setRequiredFields([]);
		setError(null);
	};

	const createMutation = useMutation({
		mutationFn: () => {
			if (!toStatusId) return Promise.reject(new Error("no to status"));
			return createStatusTransition(projectId, {
				task_type_id: taskTypeId === ALL_TYPES ? null : taskTypeId,
				from_status_id: fromStatusId === ANY_STATUS ? null : fromStatusId,
				to_status_id: toStatusId,
				required_fields: requiredFields,
			});
		},
		onSuccess: () => {
			void queryClient.invalidateQueries({
				queryKey: statusTransitionsQueryOptions(projectId).queryKey,
			});
			resetForm();
		},
		onError: (err: unknown) => {
			setError(
				isConflict(err)
					? t("settings.workflow.errors.duplicate")
					: t("settings.workflow.errors.createFailed"),
			);
		},
	});

	const deleteMutation = useMutation({
		mutationFn: (transitionId: string) =>
			deleteStatusTransition(projectId, transitionId),
		onSuccess: () => {
			void queryClient.invalidateQueries({
				queryKey: statusTransitionsQueryOptions(projectId).queryKey,
			});
		},
		onError: () => setError(t("settings.workflow.errors.deleteFailed")),
	});

	const toggleRequiredField = (key: string) => {
		setRequiredFields((prev) =>
			prev.includes(key) ? prev.filter((k) => k !== key) : [...prev, key],
		);
	};

	// The backend rejects a same-from/to rule; guard it in the UI too.
	const sameFromTo = fromStatusId !== ANY_STATUS && fromStatusId === toStatusId;
	const canSubmit = !!toStatusId && !sameFromTo && !createMutation.isPending;

	return (
		<div className="rounded-xl border border-border/60 bg-card p-6">
			<div className="mb-1">
				<h3 className="font-[Syne] text-base font-semibold">
					{t("settings.workflow.title")}
				</h3>
				<p className="text-xs text-muted-foreground mt-0.5 max-w-md">
					{t("settings.workflow.description")}
				</p>
			</div>

			{/* Free-movement explainer — enforcement only starts once a rule exists. */}
			<div className="mt-4 rounded-lg border border-border/40 bg-muted/20 px-3 py-2.5 text-xs text-muted-foreground">
				{t("settings.workflow.freeMovementNote")}
			</div>

			{error ? (
				<p className="mt-4 text-xs text-destructive bg-destructive/10 rounded-lg px-3 py-2">
					{error}
				</p>
			) : null}

			{/* Add-transition form */}
			{canWrite && sortedStatuses.length > 0 ? (
				<div className="mt-5 rounded-xl border border-border/50 bg-muted/10 p-4 space-y-4">
					<div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
						<div className="space-y-1.5">
							<Label>{t("settings.workflow.fromLabel")}</Label>
							<Select
								value={fromStatusId}
								onValueChange={(v) => setFromStatusId(v ?? ANY_STATUS)}
								items={[
									{
										value: ANY_STATUS,
										label: t("settings.workflow.anyStatus"),
									},
									...sortedStatuses.map((s) => ({
										value: s.id,
										label: s.name,
									})),
								]}
							>
								<SelectTrigger className="w-full">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value={ANY_STATUS}>
										{t("settings.workflow.anyStatus")}
									</SelectItem>
									{sortedStatuses.map((s) => (
										<SelectItem key={s.id} value={s.id}>
											{s.name}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>

						<div className="space-y-1.5">
							<Label>{t("settings.workflow.toLabel")}</Label>
							<Select
								value={toStatusId}
								onValueChange={(v) => setToStatusId(v)}
								items={sortedStatuses.map((s) => ({
									value: s.id,
									label: s.name,
								}))}
							>
								<SelectTrigger className="w-full">
									<SelectValue
										placeholder={t("settings.workflow.selectToStatus")}
									/>
								</SelectTrigger>
								<SelectContent>
									{sortedStatuses.map((s) => (
										<SelectItem key={s.id} value={s.id}>
											{s.name}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>

						<div className="space-y-1.5">
							<Label>{t("settings.workflow.taskTypeLabel")}</Label>
							<Select
								value={taskTypeId}
								onValueChange={(v) => setTaskTypeId(v ?? ALL_TYPES)}
								items={[
									{ value: ALL_TYPES, label: t("settings.workflow.allTypes") },
									...taskTypes.map((tt) => ({ value: tt.id, label: tt.name })),
								]}
							>
								<SelectTrigger className="w-full">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value={ALL_TYPES}>
										{t("settings.workflow.allTypes")}
									</SelectItem>
									{taskTypes.map((tt) => (
										<SelectItem key={tt.id} value={tt.id}>
											{tt.name}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>
					</div>

					{customFields.length > 0 ? (
						<div className="space-y-1.5">
							<Label>{t("settings.workflow.requiredFieldsLabel")}</Label>
							<div className="flex flex-wrap gap-1.5">
								{customFields.map((cf) => {
									const on = requiredFields.includes(cf.field_key);
									return (
										<button
											key={cf.id}
											type="button"
											onClick={() => toggleRequiredField(cf.field_key)}
											className={cn(
												PILL_BASE,
												on ? PILL_SELECTED : PILL_UNSELECTED,
											)}
										>
											{cf.display_name}
										</button>
									);
								})}
							</div>
							<p className="text-xs text-muted-foreground/60">
								{t("settings.workflow.requiredFieldsHint")}
							</p>
						</div>
					) : null}

					<div className="flex justify-end">
						<Button
							size="sm"
							disabled={!canSubmit}
							onClick={() => createMutation.mutate()}
						>
							{createMutation.isPending ? (
								<Loader2 className="size-3.5 animate-spin" />
							) : (
								<Plus className="size-3.5" />
							)}
							{createMutation.isPending
								? t("settings.workflow.adding")
								: t("settings.workflow.addTransition")}
						</Button>
					</div>
				</div>
			) : null}

			{/* Transitions list */}
			{isLoading ? (
				<div className="mt-4 rounded-xl border overflow-hidden">
					{["w1", "w2", "w3"].map((k) => (
						<div
							key={k}
							className="flex items-center gap-4 border-b px-5 py-4 last:border-0"
						>
							<Skeleton className="h-4 w-40" />
							<Skeleton className="h-4 w-20" />
							<Skeleton className="h-4 w-24 ml-auto" />
						</div>
					))}
				</div>
			) : transitions.length === 0 ? (
				<div className="mt-4 flex flex-col items-center gap-3 rounded-xl border border-dashed border-border/60 bg-muted/10 py-12 text-center">
					<div className="flex size-11 items-center justify-center rounded-xl bg-muted">
						<GitBranch className="size-5 text-muted-foreground/60" />
					</div>
					<div>
						<p className="text-sm font-medium">
							{t("settings.workflow.empty.title")}
						</p>
						<p className="mt-1 text-xs text-muted-foreground max-w-xs mx-auto">
							{t("settings.workflow.empty.description")}
						</p>
					</div>
				</div>
			) : (
				<div className="mt-4 overflow-x-auto rounded-xl border">
					<Table>
						<TableHeader>
							<TableRow className="bg-muted/40 hover:bg-muted/40">
								<TableHead className="px-5 text-xs font-semibold uppercase tracking-wide">
									{t("settings.workflow.table.transition")}
								</TableHead>
								<TableHead className="px-5 text-xs font-semibold uppercase tracking-wide">
									{t("settings.workflow.table.taskType")}
								</TableHead>
								<TableHead className="px-5 text-xs font-semibold uppercase tracking-wide">
									{t("settings.workflow.table.requiredFields")}
								</TableHead>
								{canWrite && <TableHead className="w-16 px-5" />}
							</TableRow>
						</TableHeader>
						<TableBody>
							{transitions.map((tr: StatusTransition) => (
								<TableRow key={tr.id} className="group">
									<TableCell className="px-5">
										<div className="flex items-center gap-2 text-sm font-medium">
											<span>
												{statusName(tr.from_status_id) ??
													t("settings.workflow.anyStatus")}
											</span>
											<ArrowRight className="size-3.5 shrink-0 text-muted-foreground/60" />
											<span>{statusName(tr.to_status_id)}</span>
										</div>
									</TableCell>
									<TableCell className="px-5 text-sm text-muted-foreground">
										{typeName(tr.task_type_id) ??
											t("settings.workflow.allTypes")}
									</TableCell>
									<TableCell className="px-5">
										{tr.required_fields.length > 0 ? (
											<div className="flex flex-wrap gap-1">
												{tr.required_fields.map((key) => (
													<span
														key={key}
														className="inline-flex items-center rounded-md border border-border/40 bg-muted/40 px-2 py-0.5 text-xs font-medium text-muted-foreground"
													>
														{fieldName(key)}
													</span>
												))}
											</div>
										) : (
											<span className="text-xs text-muted-foreground/50">
												{t("settings.workflow.noRequiredFields")}
											</span>
										)}
									</TableCell>
									{canWrite && (
										<TableCell className="px-5">
											<div className="flex items-center justify-end opacity-100 transition-opacity sm:opacity-0 sm:group-hover:opacity-100">
												<Button
													variant="ghost"
													size="icon-sm"
													className="text-destructive hover:text-destructive hover:bg-destructive/10"
													disabled={deleteMutation.isPending}
													onClick={() => deleteMutation.mutate(tr.id)}
													title={t("settings.workflow.deleteTransition")}
													aria-label={t("settings.workflow.deleteTransition")}
												>
													<Trash2 className="size-3.5" />
												</Button>
											</div>
										</TableCell>
									)}
								</TableRow>
							))}
						</TableBody>
					</Table>
				</div>
			)}
		</div>
	);
}
