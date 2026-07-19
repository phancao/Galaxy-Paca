import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, Loader2 } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select";
import {
	copyProjectConfiguration,
	customFieldsQueryOptions,
	projectsQueryOptions,
	statusTransitionsQueryOptions,
	taskStatusesQueryOptions,
	taskTypesQueryOptions,
} from "@/lib/project-api";
import { cn } from "@/lib/utils";

/**
 * "Copy configuration" card — pulls task types, statuses, custom fields, and
 * workflow transitions from another project the user belongs to into this one.
 * The copy is additive/idempotent (see {@link copyProjectConfiguration}); the
 * confirm step spells that out before the request fires. Gated on `canWrite`.
 */
export function CopyConfigSettings({
	projectId,
	canWrite,
}: {
	projectId: string;
	canWrite: boolean;
}) {
	const { t } = useTranslation("projects");
	const queryClient = useQueryClient();
	const { data: projectList } = useQuery(projectsQueryOptions());

	const [sourceId, setSourceId] = useState<string | null>(null);
	const [confirmOpen, setConfirmOpen] = useState(false);
	const [status, setStatus] = useState<{
		type: "success" | "error";
		message: string;
	} | null>(null);

	// Exclude the current project — you can only copy FROM another project.
	const otherProjects = (projectList?.items ?? []).filter(
		(p) => p.id !== projectId,
	);
	const sourceName = otherProjects.find((p) => p.id === sourceId)?.name ?? "";

	const mutation = useMutation({
		mutationFn: () => {
			if (!sourceId) return Promise.reject(new Error("no source project"));
			return copyProjectConfiguration(projectId, sourceId);
		},
		onSuccess: () => {
			// Refresh every config surface so the newly-copied rows show up.
			for (const opts of [
				taskTypesQueryOptions(projectId),
				taskStatusesQueryOptions(projectId),
				customFieldsQueryOptions(projectId),
				statusTransitionsQueryOptions(projectId),
			]) {
				void queryClient.invalidateQueries({ queryKey: opts.queryKey });
			}
			setStatus({
				type: "success",
				message: t("settings.copyConfig.success", { project: sourceName }),
			});
			setConfirmOpen(false);
			setSourceId(null);
		},
		onError: () => {
			setStatus({ type: "error", message: t("settings.copyConfig.error") });
			setConfirmOpen(false);
		},
	});

	if (!canWrite) return null;

	return (
		<div className="rounded-xl border border-border/60 bg-card p-6">
			<div className="mb-1">
				<h3 className="font-[Syne] text-base font-semibold">
					{t("settings.copyConfig.title")}
				</h3>
				<p className="text-xs text-muted-foreground mt-0.5 max-w-md">
					{t("settings.copyConfig.description")}
				</p>
			</div>

			{otherProjects.length === 0 ? (
				<p className="mt-4 rounded-lg border border-border/40 bg-muted/20 px-3 py-2.5 text-xs text-muted-foreground">
					{t("settings.copyConfig.noProjects")}
				</p>
			) : (
				<div className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-end">
					<div className="flex-1 space-y-1.5">
						<Label>{t("settings.copyConfig.sourceLabel")}</Label>
						<Select
							value={sourceId}
							onValueChange={(v) => setSourceId(v)}
							items={otherProjects.map((p) => ({
								value: p.id,
								label: p.name,
							}))}
						>
							<SelectTrigger className="w-full">
								<SelectValue
									placeholder={t("settings.copyConfig.sourcePlaceholder")}
								/>
							</SelectTrigger>
							<SelectContent>
								{otherProjects.map((p) => (
									<SelectItem key={p.id} value={p.id}>
										{p.name}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>
					<Button
						variant="outline"
						className="gap-1.5 border-border/60 shrink-0"
						disabled={!sourceId || mutation.isPending}
						onClick={() => {
							setStatus(null);
							setConfirmOpen(true);
						}}
					>
						<Copy className="size-3.5" />
						{t("settings.copyConfig.copyButton")}
					</Button>
				</div>
			)}

			<p className="mt-3 text-xs text-muted-foreground/70">
				{t("settings.copyConfig.hint")}
			</p>

			{status ? (
				<p
					className={cn(
						"mt-3 text-xs rounded-lg px-3 py-2",
						status.type === "success"
							? "text-emerald-600 bg-emerald-500/10"
							: "text-destructive bg-destructive/10",
					)}
				>
					{status.message}
				</p>
			) : null}

			{/* Confirm step — spell out that the copy is additive / idempotent. */}
			<Dialog
				open={confirmOpen}
				onOpenChange={(o) => {
					if (!mutation.isPending) setConfirmOpen(o);
				}}
			>
				<DialogContent className="sm:max-w-md">
					<DialogHeader>
						<DialogTitle>{t("settings.copyConfig.confirm.title")}</DialogTitle>
						<DialogDescription>
							{t("settings.copyConfig.confirm.description", {
								project: sourceName,
							})}
						</DialogDescription>
					</DialogHeader>

					<ul className="list-disc space-y-1.5 pl-5 text-xs text-muted-foreground">
						<li>{t("settings.copyConfig.confirm.additive")}</li>
						<li>{t("settings.copyConfig.confirm.idempotent")}</li>
						<li>{t("settings.copyConfig.confirm.nonDestructive")}</li>
					</ul>

					<DialogFooter>
						<DialogClose
							render={
								<Button
									variant="outline"
									size="sm"
									disabled={mutation.isPending}
								/>
							}
						>
							{t("settings.copyConfig.confirm.cancel")}
						</DialogClose>
						<Button
							size="sm"
							disabled={mutation.isPending}
							onClick={() => mutation.mutate()}
						>
							{mutation.isPending ? (
								<Loader2 className="size-3.5 animate-spin" />
							) : null}
							{t("settings.copyConfig.confirm.confirmButton")}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}
