import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Edit2, Loader2, Milestone, Plus, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@/components/ui/table";
import { formatDate } from "@/lib/format-date";
import {
	createProjectVersion,
	deleteProjectVersion,
	type ProjectVersion,
	projectVersionsQueryOptions,
	updateProjectVersion,
} from "@/lib/project-api";
import { cn } from "@/lib/utils";

function Toggle({
	checked,
	onChange,
}: {
	checked: boolean;
	onChange: (v: boolean) => void;
}) {
	return (
		<button
			type="button"
			role="switch"
			aria-checked={checked}
			onClick={() => onChange(!checked)}
			className={cn(
				"relative inline-flex h-5 w-9 items-center rounded-full border-2 transition-colors",
				checked ? "border-primary bg-primary" : "border-border bg-muted",
			)}
		>
			<span
				className={cn(
					"inline-block size-3.5 rounded-full bg-white shadow transition-transform",
					checked ? "translate-x-4" : "translate-x-0.5",
				)}
			/>
		</button>
	);
}

// ── Form dialog (create + edit) ───────────────────────────────────────────────

function VersionFormDialog({
	projectId,
	version,
	open,
	onOpenChange,
}: {
	projectId: string;
	version?: ProjectVersion | null;
	open: boolean;
	onOpenChange: (v: boolean) => void;
}) {
	const { t } = useTranslation("projects");
	const queryClient = useQueryClient();
	const isEdit = !!version;

	const [name, setName] = useState(version?.name ?? "");
	const [description, setDescription] = useState(version?.description ?? "");
	const [released, setReleased] = useState(version?.released ?? false);
	const [releaseDate, setReleaseDate] = useState(
		version?.release_date ? version.release_date.slice(0, 10) : "",
	);
	const [archived, setArchived] = useState(version?.archived ?? false);
	const [error, setError] = useState<string | null>(null);

	useEffect(() => {
		setName(version?.name ?? "");
		setDescription(version?.description ?? "");
		setReleased(version?.released ?? false);
		setReleaseDate(
			version?.release_date ? version.release_date.slice(0, 10) : "",
		);
		setArchived(version?.archived ?? false);
		setError(null);
	}, [version]);

	const mutation = useMutation({
		mutationFn: () => {
			const payload = {
				name: name.trim(),
				description: description.trim(),
				released,
				release_date: releaseDate || null,
				archived,
			};
			return isEdit
				? updateProjectVersion(projectId, version.id, payload)
				: createProjectVersion(projectId, payload);
		},
		onSuccess: () => {
			void queryClient.invalidateQueries({
				queryKey: projectVersionsQueryOptions(projectId).queryKey,
			});
			onOpenChange(false);
		},
		onError: () =>
			setError(
				isEdit
					? t("settings.versions.errors.saveFailed")
					: t("settings.versions.errors.createFailed"),
			),
	});

	return (
		<Dialog
			open={open}
			onOpenChange={(o) => {
				if (!o) setError(null);
				onOpenChange(o);
			}}
		>
			<DialogContent className="sm:max-w-md">
				<DialogHeader>
					<DialogTitle>
						{isEdit
							? t("settings.versions.editDialog.title")
							: t("settings.versions.createDialog.title")}
					</DialogTitle>
					<DialogDescription>
						{isEdit
							? t("settings.versions.editDialog.description")
							: t("settings.versions.createDialog.description")}
					</DialogDescription>
				</DialogHeader>

				<div className="space-y-4">
					<div className="space-y-1.5">
						<Label htmlFor="version-name">
							{t("settings.versions.nameLabel")}{" "}
							<span className="text-destructive">*</span>
						</Label>
						<Input
							id="version-name"
							value={name}
							onChange={(e) => setName(e.target.value)}
							placeholder={t("settings.versions.namePlaceholder")}
							autoFocus
						/>
					</div>

					<div className="space-y-1.5">
						<Label htmlFor="version-description">
							{t("settings.versions.descriptionLabel")}
						</Label>
						<Textarea
							id="version-description"
							value={description}
							onChange={(e) => setDescription(e.target.value)}
							placeholder={t("settings.versions.descriptionPlaceholder")}
							rows={2}
						/>
					</div>

					<div className="space-y-1.5">
						<Label htmlFor="version-release-date">
							{t("settings.versions.releaseDateLabel")}
						</Label>
						<Input
							id="version-release-date"
							type="date"
							value={releaseDate}
							onChange={(e) => setReleaseDate(e.target.value)}
						/>
					</div>

					<div className="flex items-center justify-between rounded-xl border border-border/50 bg-muted/20 px-4 py-3">
						<div>
							<p className="text-sm font-medium">
								{t("settings.versions.releasedLabel")}
							</p>
							<p className="text-xs text-muted-foreground/70">
								{t("settings.versions.releasedHint")}
							</p>
						</div>
						<Toggle checked={released} onChange={setReleased} />
					</div>

					<div className="flex items-center justify-between rounded-xl border border-border/50 bg-muted/20 px-4 py-3">
						<div>
							<p className="text-sm font-medium">
								{t("settings.versions.archivedLabel")}
							</p>
							<p className="text-xs text-muted-foreground/70">
								{t("settings.versions.archivedHint")}
							</p>
						</div>
						<Toggle checked={archived} onChange={setArchived} />
					</div>

					{error ? (
						<p className="text-xs text-destructive bg-destructive/10 rounded-lg px-3 py-2">
							{error}
						</p>
					) : null}
				</div>

				<DialogFooter>
					<DialogClose
						render={
							<Button variant="outline" size="sm" disabled={mutation.isPending} />
						}
					>
						{t("settings.versions.cancel")}
					</DialogClose>
					<Button
						size="sm"
						disabled={!name.trim() || mutation.isPending}
						onClick={() => mutation.mutate()}
					>
						{mutation.isPending ? (
							<Loader2 className="size-3.5 animate-spin" />
						) : null}
						{isEdit
							? t("settings.versions.saveChanges")
							: t("settings.versions.createDialog.createVersion")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

// ── Delete dialog ─────────────────────────────────────────────────────────────

function DeleteVersionDialog({
	projectId,
	version,
	open,
	onOpenChange,
}: {
	projectId: string;
	version: ProjectVersion | null;
	open: boolean;
	onOpenChange: (v: boolean) => void;
}) {
	const { t } = useTranslation("projects");
	const queryClient = useQueryClient();
	const [error, setError] = useState<string | null>(null);

	const mutation = useMutation({
		mutationFn: () => {
			if (!version) return Promise.resolve();
			return deleteProjectVersion(projectId, version.id);
		},
		onSuccess: () => {
			void queryClient.invalidateQueries({
				queryKey: projectVersionsQueryOptions(projectId).queryKey,
			});
			onOpenChange(false);
		},
		onError: () => setError(t("settings.versions.deleteDialog.deleteFailed")),
	});

	if (!version) return null;

	return (
		<Dialog
			open={open}
			onOpenChange={(o) => {
				if (!o) setError(null);
				onOpenChange(o);
			}}
		>
			<DialogContent className="sm:max-w-sm">
				<DialogHeader>
					<div className="flex size-10 items-center justify-center rounded-full bg-destructive/10 mb-2">
						<Trash2 className="size-5 text-destructive" />
					</div>
					<DialogTitle>
						{t("settings.versions.deleteDialog.title")}
					</DialogTitle>
					<DialogDescription>
						{t("settings.versions.deleteDialog.confirmTextPrefix")}{" "}
						<span className="font-semibold text-foreground">
							&ldquo;{version.name}&rdquo;
						</span>
						{t("settings.versions.deleteDialog.confirmTextSuffix")}
					</DialogDescription>
				</DialogHeader>

				{error ? (
					<p className="text-xs text-destructive bg-destructive/10 rounded-lg px-3 py-2">
						{error}
					</p>
				) : null}

				<DialogFooter>
					<DialogClose
						render={
							<Button variant="outline" size="sm" disabled={mutation.isPending} />
						}
					>
						{t("settings.versions.cancel")}
					</DialogClose>
					<Button
						variant="destructive"
						size="sm"
						disabled={mutation.isPending}
						onClick={() => mutation.mutate()}
					>
						{mutation.isPending ? (
							<Loader2 className="size-3.5 animate-spin" />
						) : null}
						{t("settings.versions.deleteDialog.deleteVersion")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

// ── Main section ──────────────────────────────────────────────────────────────

export function VersionsSettings({
	projectId,
	canWrite,
}: {
	projectId: string;
	canWrite: boolean;
}) {
	const { t } = useTranslation("projects");
	const { data: versions = [], isLoading } = useQuery(
		projectVersionsQueryOptions(projectId),
	);
	const [createOpen, setCreateOpen] = useState(false);
	const [editVersion, setEditVersion] = useState<ProjectVersion | null>(null);
	const [deleteVersion, setDeleteVersion] = useState<ProjectVersion | null>(
		null,
	);

	return (
		<div className="rounded-xl border border-border/60 bg-card p-6">
			<div className="flex items-start justify-between mb-1">
				<div>
					<h3 className="font-[Syne] text-base font-semibold">
						{t("settings.versions.title")}
					</h3>
					<p className="text-xs text-muted-foreground mt-0.5 max-w-xs">
						{t("settings.versions.description")}
					</p>
				</div>
				{canWrite && (
					<Button
						size="sm"
						variant="outline"
						className="gap-1.5 border-border/60 shrink-0"
						onClick={() => setCreateOpen(true)}
					>
						<Plus className="size-3.5" />
						{t("settings.versions.newVersion")}
					</Button>
				)}
			</div>

			{isLoading ? (
				<div className="rounded-xl border overflow-hidden mt-4">
					{["v1", "v2", "v3"].map((k) => (
						<div
							key={k}
							className="flex items-center gap-4 border-b px-5 py-4 last:border-0"
						>
							<Skeleton className="h-4 w-40" />
							<Skeleton className="h-5 w-16 rounded-md" />
							<Skeleton className="h-4 w-24 ml-auto" />
						</div>
					))}
				</div>
			) : versions.length === 0 ? (
				<div className="mt-4 flex flex-col items-center gap-4 rounded-xl border border-dashed border-border/60 bg-muted/10 py-14 text-center">
					<div className="flex size-11 items-center justify-center rounded-xl bg-muted">
						<Milestone className="size-5 text-muted-foreground/60" />
					</div>
					<div>
						<p className="text-sm font-medium">
							{t("settings.versions.empty.title")}
						</p>
						<p className="mt-1 text-xs text-muted-foreground max-w-xs mx-auto">
							{t("settings.versions.empty.description")}
						</p>
					</div>
					{canWrite && (
						<Button size="sm" variant="outline" onClick={() => setCreateOpen(true)}>
							<Plus className="size-4 mr-1" />
							{t("settings.versions.empty.createFirst")}
						</Button>
					)}
				</div>
			) : (
				<div className="mt-4 overflow-x-auto rounded-xl border">
					<Table>
						<TableHeader>
							<TableRow className="bg-muted/40 hover:bg-muted/40">
								<TableHead className="px-5 text-xs font-semibold uppercase tracking-wide">
									{t("settings.versions.table.name")}
								</TableHead>
								<TableHead className="px-5 text-xs font-semibold uppercase tracking-wide">
									{t("settings.versions.table.status")}
								</TableHead>
								<TableHead className="px-5 text-xs font-semibold uppercase tracking-wide">
									{t("settings.versions.table.releaseDate")}
								</TableHead>
								{canWrite && <TableHead className="w-20 px-5" />}
							</TableRow>
						</TableHeader>
						<TableBody>
							{versions.map((v) => (
								<TableRow key={v.id} className="group">
									<TableCell className="px-5">
										<div className="flex items-center gap-2">
											<span className="font-medium">{v.name}</span>
											{v.archived && (
												<span className="rounded-md border border-border/40 bg-muted/40 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
													{t("settings.versions.archivedBadge")}
												</span>
											)}
										</div>
										{v.description ? (
											<p className="mt-0.5 text-xs text-muted-foreground line-clamp-1">
												{v.description}
											</p>
										) : null}
									</TableCell>
									<TableCell className="px-5">
										{v.released ? (
											<span className="inline-flex items-center gap-1 rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400">
												<Check className="size-3" />
												{t("settings.versions.released")}
											</span>
										) : (
											<span className="text-xs text-muted-foreground/60">
												{t("settings.versions.unreleased")}
											</span>
										)}
									</TableCell>
									<TableCell className="px-5 text-sm text-muted-foreground">
										{v.release_date
											? formatDate(v.release_date)
											: t("settings.versions.noDate")}
									</TableCell>
									{canWrite && (
										<TableCell className="px-5">
											<div className="flex items-center justify-end gap-0.5 opacity-100 transition-opacity sm:opacity-0 sm:group-hover:opacity-100">
												<Button
													variant="ghost"
													size="icon-sm"
													onClick={() => setEditVersion(v)}
													title={t("settings.versions.editVersion")}
													aria-label={t("settings.versions.editVersion")}
												>
													<Edit2 className="size-3.5" />
												</Button>
												<Button
													variant="ghost"
													size="icon-sm"
													className="text-destructive hover:text-destructive hover:bg-destructive/10"
													onClick={() => setDeleteVersion(v)}
													title={t("settings.versions.deleteVersion")}
													aria-label={t("settings.versions.deleteVersion")}
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

			<VersionFormDialog
				projectId={projectId}
				open={createOpen}
				onOpenChange={setCreateOpen}
			/>
			<VersionFormDialog
				projectId={projectId}
				version={editVersion}
				open={!!editVersion}
				onOpenChange={(o) => {
					if (!o) setEditVersion(null);
				}}
			/>
			<DeleteVersionDialog
				projectId={projectId}
				version={deleteVersion}
				open={!!deleteVersion}
				onOpenChange={(o) => {
					if (!o) setDeleteVersion(null);
				}}
			/>
		</div>
	);
}
