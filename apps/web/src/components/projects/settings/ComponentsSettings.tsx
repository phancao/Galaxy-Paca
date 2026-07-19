import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Edit2, Loader2, Package, Plus, Trash2 } from "lucide-react";
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
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select";
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
import {
	createProjectComponent,
	deleteProjectComponent,
	type ProjectComponent,
	type ProjectMember,
	projectComponentsQueryOptions,
	projectMembersQueryOptions,
	updateProjectComponent,
} from "@/lib/project-api";

// Sentinel for the "no lead" option (lead_member_id = null).
const NO_LEAD = "__none__";

const memberLabel = (m: ProjectMember) =>
	m.full_name || m.username || m.agent_name || m.id;

// ── Form dialog (create + edit) ───────────────────────────────────────────────

function ComponentFormDialog({
	projectId,
	members,
	component,
	open,
	onOpenChange,
}: {
	projectId: string;
	members: ProjectMember[];
	component?: ProjectComponent | null;
	open: boolean;
	onOpenChange: (v: boolean) => void;
}) {
	const { t } = useTranslation("projects");
	const queryClient = useQueryClient();
	const isEdit = !!component;

	const [name, setName] = useState(component?.name ?? "");
	const [description, setDescription] = useState(component?.description ?? "");
	const [leadId, setLeadId] = useState<string>(
		component?.lead_member_id ?? NO_LEAD,
	);
	const [error, setError] = useState<string | null>(null);

	useEffect(() => {
		setName(component?.name ?? "");
		setDescription(component?.description ?? "");
		setLeadId(component?.lead_member_id ?? NO_LEAD);
		setError(null);
	}, [component]);

	const mutation = useMutation({
		mutationFn: () => {
			const payload = {
				name: name.trim(),
				description: description.trim(),
				lead_member_id: leadId === NO_LEAD ? null : leadId,
			};
			return isEdit
				? updateProjectComponent(projectId, component.id, payload)
				: createProjectComponent(projectId, payload);
		},
		onSuccess: () => {
			void queryClient.invalidateQueries({
				queryKey: projectComponentsQueryOptions(projectId).queryKey,
			});
			onOpenChange(false);
		},
		onError: () =>
			setError(
				isEdit
					? t("settings.components.errors.saveFailed")
					: t("settings.components.errors.createFailed"),
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
							? t("settings.components.editDialog.title")
							: t("settings.components.createDialog.title")}
					</DialogTitle>
					<DialogDescription>
						{isEdit
							? t("settings.components.editDialog.description")
							: t("settings.components.createDialog.description")}
					</DialogDescription>
				</DialogHeader>

				<div className="space-y-4">
					<div className="space-y-1.5">
						<Label htmlFor="component-name">
							{t("settings.components.nameLabel")}{" "}
							<span className="text-destructive">*</span>
						</Label>
						<Input
							id="component-name"
							value={name}
							onChange={(e) => setName(e.target.value)}
							placeholder={t("settings.components.namePlaceholder")}
							autoFocus
						/>
					</div>

					<div className="space-y-1.5">
						<Label htmlFor="component-description">
							{t("settings.components.descriptionLabel")}
						</Label>
						<Textarea
							id="component-description"
							value={description}
							onChange={(e) => setDescription(e.target.value)}
							placeholder={t("settings.components.descriptionPlaceholder")}
							rows={2}
						/>
					</div>

					<div className="space-y-1.5">
						<Label>{t("settings.components.leadLabel")}</Label>
						<Select
							value={leadId}
							onValueChange={(v) => setLeadId(v ?? NO_LEAD)}
							items={[
								{ value: NO_LEAD, label: t("settings.components.noLead") },
								...members.map((m) => ({ value: m.id, label: memberLabel(m) })),
							]}
						>
							<SelectTrigger className="w-full">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value={NO_LEAD}>
									{t("settings.components.noLead")}
								</SelectItem>
								{members.map((m) => (
									<SelectItem key={m.id} value={m.id}>
										{memberLabel(m)}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
						<p className="text-xs text-muted-foreground/60">
							{t("settings.components.leadHint")}
						</p>
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
						{t("settings.components.cancel")}
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
							? t("settings.components.saveChanges")
							: t("settings.components.createDialog.createComponent")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

// ── Delete dialog ─────────────────────────────────────────────────────────────

function DeleteComponentDialog({
	projectId,
	component,
	open,
	onOpenChange,
}: {
	projectId: string;
	component: ProjectComponent | null;
	open: boolean;
	onOpenChange: (v: boolean) => void;
}) {
	const { t } = useTranslation("projects");
	const queryClient = useQueryClient();
	const [error, setError] = useState<string | null>(null);

	const mutation = useMutation({
		mutationFn: () => {
			if (!component) return Promise.resolve();
			return deleteProjectComponent(projectId, component.id);
		},
		onSuccess: () => {
			void queryClient.invalidateQueries({
				queryKey: projectComponentsQueryOptions(projectId).queryKey,
			});
			onOpenChange(false);
		},
		onError: () => setError(t("settings.components.deleteDialog.deleteFailed")),
	});

	if (!component) return null;

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
						{t("settings.components.deleteDialog.title")}
					</DialogTitle>
					<DialogDescription>
						{t("settings.components.deleteDialog.confirmTextPrefix")}{" "}
						<span className="font-semibold text-foreground">
							&ldquo;{component.name}&rdquo;
						</span>
						{t("settings.components.deleteDialog.confirmTextSuffix")}
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
						{t("settings.components.cancel")}
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
						{t("settings.components.deleteDialog.deleteComponent")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

// ── Main section ──────────────────────────────────────────────────────────────

export function ComponentsSettings({
	projectId,
	canWrite,
}: {
	projectId: string;
	canWrite: boolean;
}) {
	const { t } = useTranslation("projects");
	const { data: components = [], isLoading } = useQuery(
		projectComponentsQueryOptions(projectId),
	);
	const { data: members = [] } = useQuery(
		projectMembersQueryOptions(projectId),
	);
	const memberName = (id: string) => {
		const m = members.find((mm) => mm.id === id);
		return m ? memberLabel(m) : id;
	};

	const [createOpen, setCreateOpen] = useState(false);
	const [editComponent, setEditComponent] = useState<ProjectComponent | null>(
		null,
	);
	const [deleteComponent, setDeleteComponent] =
		useState<ProjectComponent | null>(null);

	return (
		<div className="rounded-xl border border-border/60 bg-card p-6">
			<div className="flex items-start justify-between mb-1">
				<div>
					<h3 className="font-[Syne] text-base font-semibold">
						{t("settings.components.title")}
					</h3>
					<p className="text-xs text-muted-foreground mt-0.5 max-w-xs">
						{t("settings.components.description")}
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
						{t("settings.components.newComponent")}
					</Button>
				)}
			</div>

			{isLoading ? (
				<div className="rounded-xl border overflow-hidden mt-4">
					{["c1", "c2", "c3"].map((k) => (
						<div
							key={k}
							className="flex items-center gap-4 border-b px-5 py-4 last:border-0"
						>
							<Skeleton className="h-4 w-40" />
							<Skeleton className="h-4 w-24 ml-auto" />
						</div>
					))}
				</div>
			) : components.length === 0 ? (
				<div className="mt-4 flex flex-col items-center gap-4 rounded-xl border border-dashed border-border/60 bg-muted/10 py-14 text-center">
					<div className="flex size-11 items-center justify-center rounded-xl bg-muted">
						<Package className="size-5 text-muted-foreground/60" />
					</div>
					<div>
						<p className="text-sm font-medium">
							{t("settings.components.empty.title")}
						</p>
						<p className="mt-1 text-xs text-muted-foreground max-w-xs mx-auto">
							{t("settings.components.empty.description")}
						</p>
					</div>
					{canWrite && (
						<Button size="sm" variant="outline" onClick={() => setCreateOpen(true)}>
							<Plus className="size-4 mr-1" />
							{t("settings.components.empty.createFirst")}
						</Button>
					)}
				</div>
			) : (
				<div className="mt-4 overflow-x-auto rounded-xl border">
					<Table>
						<TableHeader>
							<TableRow className="bg-muted/40 hover:bg-muted/40">
								<TableHead className="px-5 text-xs font-semibold uppercase tracking-wide">
									{t("settings.components.table.name")}
								</TableHead>
								<TableHead className="px-5 text-xs font-semibold uppercase tracking-wide">
									{t("settings.components.table.lead")}
								</TableHead>
								{canWrite && <TableHead className="w-20 px-5" />}
							</TableRow>
						</TableHeader>
						<TableBody>
							{components.map((c) => (
								<TableRow key={c.id} className="group">
									<TableCell className="px-5">
										<span className="font-medium">{c.name}</span>
										{c.description ? (
											<p className="mt-0.5 text-xs text-muted-foreground line-clamp-1">
												{c.description}
											</p>
										) : null}
									</TableCell>
									<TableCell className="px-5 text-sm text-muted-foreground">
										{c.lead_member_id ? (
											memberName(c.lead_member_id)
										) : (
											<span className="text-muted-foreground/50">
												{t("settings.components.noLead")}
											</span>
										)}
									</TableCell>
									{canWrite && (
										<TableCell className="px-5">
											<div className="flex items-center justify-end gap-0.5 opacity-100 transition-opacity sm:opacity-0 sm:group-hover:opacity-100">
												<Button
													variant="ghost"
													size="icon-sm"
													onClick={() => setEditComponent(c)}
													title={t("settings.components.editComponent")}
													aria-label={t("settings.components.editComponent")}
												>
													<Edit2 className="size-3.5" />
												</Button>
												<Button
													variant="ghost"
													size="icon-sm"
													className="text-destructive hover:text-destructive hover:bg-destructive/10"
													onClick={() => setDeleteComponent(c)}
													title={t("settings.components.deleteComponent")}
													aria-label={t("settings.components.deleteComponent")}
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

			<ComponentFormDialog
				projectId={projectId}
				members={members}
				open={createOpen}
				onOpenChange={setCreateOpen}
			/>
			<ComponentFormDialog
				projectId={projectId}
				members={members}
				component={editComponent}
				open={!!editComponent}
				onOpenChange={(o) => {
					if (!o) setEditComponent(null);
				}}
			/>
			<DeleteComponentDialog
				projectId={projectId}
				component={deleteComponent}
				open={!!deleteComponent}
				onOpenChange={(o) => {
					if (!o) setDeleteComponent(null);
				}}
			/>
		</div>
	);
}
