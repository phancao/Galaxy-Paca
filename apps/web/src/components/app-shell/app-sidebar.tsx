import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	Link,
	useNavigate,
	useParams,
	useRouterState,
} from "@tanstack/react-router";
import {
	AlertTriangle,
	ArrowLeft,
	BookOpen,
	ChevronDown,
	ChevronRight,
	FileText,
	FolderKanban,
	GanttChart,
	Gauge,
	GitBranch,
	Home,
	KanbanSquare,
	LayoutList,
	Loader2,
	Milestone,
	Monitor,
	Moon,
	Package,
	Pencil,
	Plus,
	Puzzle,
	Settings,
	Shield,
	Sun,
	Tag,
	Trash2,
	Users,
	Workflow,
} from "lucide-react";
import { type ComponentType, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuGroup,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
	Sidebar,
	SidebarContent,
	SidebarFooter,
	SidebarGroup,
	SidebarGroupContent,
	SidebarGroupLabel,
	SidebarHeader,
	SidebarMenu,
	SidebarMenuButton,
	SidebarMenuItem,
	SidebarMenuSub,
	SidebarMenuSubButton,
	SidebarMenuSubItem,
	SidebarRail,
	SidebarSeparator,
	useSidebar,
} from "@/components/ui/sidebar";
import { usePermissions } from "@/hooks/use-permissions";
import { useProjectPermissions } from "@/hooks/use-project-permissions";
import type { ThemeMode } from "@/hooks/use-theme-mode";
import { useThemeMode } from "@/hooks/use-theme-mode";
import { currentUserOptionalQueryOptions } from "@/lib/auth-api";
import { sprintsQueryOptions, updateTask } from "@/lib/interaction-api";
import type { PluginNavRegistration } from "@/lib/plugin-api";
import { ExtensionPoint } from "@/lib/plugins/extension-point";
import { resolvePluginIcon } from "@/lib/plugins/icon-resolver";
import { usePluginRegistry } from "@/lib/plugins/registry";
import { projectQueryOptions, projectsQueryOptions } from "@/lib/project-api";
import { cn } from "@/lib/utils";
import {
	createWikiPage,
	deleteWikiPage,
	renameWikiPage,
	type WikiPage,
	wikiQueryKeys,
	wikiSpaceQueryOptions,
	wikiTreeQueryOptions,
} from "@/lib/wiki-api";
import { UserMenu } from "./user-menu";

// ── Documentation (Galaxy AI Wiki, ADR-042) ──────────────────────────────────

/**
 * The Documentation section is backed by the project's Galaxy AI Wiki space
 * (the native doc feature was removed by ADR-042 Stage 6). While the Wiki
 * integration is unreachable the section shows a muted unavailable note.
 */
function DocsSectionSwitch({ projectId }: { projectId: string }) {
	const { t } = useTranslation("appShell");
	const probe = useQuery(wikiSpaceQueryOptions(projectId));
	if (probe.isSuccess) return <WikiDocsSidebarSection projectId={projectId} />;
	if (probe.isPending) return null;
	return (
		<SidebarGroup className="px-0">
			<SidebarGroupLabel className="px-3">
				{t("docs.documentation")}
			</SidebarGroupLabel>
			<SidebarGroupContent>
				<p className="px-3 py-1 text-xs italic text-sidebar-foreground/45">
					{t("docs.wikiUnavailable")}
				</p>
			</SidebarGroupContent>
		</SidebarGroup>
	);
}

/**
 * Destructive-confirm dialog for deleting a wiki page from the sidebar
 * (mirrors the VersionsSettings delete dialog). Deleting moves the page to
 * the Wiki trash — recoverable there, never permanent.
 */
function WikiPageDeleteDialog({
	projectId,
	page,
	open,
	onOpenChange,
}: {
	projectId: string;
	page: WikiPage | null;
	open: boolean;
	onOpenChange: (v: boolean) => void;
}) {
	const { t } = useTranslation("appShell");
	const qc = useQueryClient();
	const [error, setError] = useState<string | null>(null);

	const mutation = useMutation({
		mutationFn: () => {
			if (!page) return Promise.resolve();
			return deleteWikiPage(projectId, page.id);
		},
		onSuccess: () => {
			void qc.invalidateQueries({ queryKey: wikiQueryKeys.tree(projectId) });
			onOpenChange(false);
		},
		onError: () =>
			setError(
				t("docs.deleteDialog.failed", {
					defaultValue: "Could not delete the page. Please try again.",
				}),
			),
	});

	if (!page) return null;

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
						{t("docs.deleteDialog.title", { defaultValue: "Delete page?" })}
					</DialogTitle>
					<DialogDescription>
						{t("docs.deleteDialog.confirmPrefix", {
							defaultValue: "The page",
						})}{" "}
						<span className="font-semibold text-foreground">
							&ldquo;{page.title || t("docs.untitled")}&rdquo;
						</span>{" "}
						{t("docs.deleteDialog.confirmSuffix", {
							defaultValue:
								"will be moved to the Wiki trash. You can restore it from there.",
						})}
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
							<Button
								variant="outline"
								size="sm"
								disabled={mutation.isPending}
							/>
						}
					>
						{t("docs.deleteDialog.cancel", { defaultValue: "Cancel" })}
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
						{t("docs.deleteDialog.delete", { defaultValue: "Delete page" })}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

/** One node of the Wiki page tree (recursive, links into the embedded view). */
function WikiPageNode({
	projectId,
	page,
	depth,
	canWrite,
	onRequestDelete,
}: {
	projectId: string;
	page: WikiPage;
	depth: number;
	canWrite: boolean;
	onRequestDelete: (page: WikiPage) => void;
}) {
	const navigate = useNavigate();
	const { t } = useTranslation("appShell");
	const qc = useQueryClient();
	const [renaming, setRenaming] = useState(false);
	const [title, setTitle] = useState(page.title);

	const invalidateTree = () =>
		qc.invalidateQueries({ queryKey: wikiQueryKeys.tree(projectId) });

	const renameMutation = useMutation({
		mutationFn: (nextTitle: string) =>
			renameWikiPage(projectId, page.id, nextTitle),
		onSuccess: () => {
			setRenaming(false);
			invalidateTree();
		},
	});

	const submitRename = () => {
		const next = title.trim();
		if (!next || next === page.title) {
			setRenaming(false);
			setTitle(page.title);
			return;
		}
		renameMutation.mutate(next);
	};

	return (
		<>
			<SidebarMenuItem className="group/wikipage">
				{renaming ? (
					<div
						className="flex h-7 items-center gap-2 pr-2"
						style={{ paddingLeft: `${12 + depth * 14}px` }}
					>
						<FileText className="size-3.5 shrink-0 text-sidebar-foreground/50" />
						<input
							// biome-ignore lint/a11y/noAutofocus: rename input should focus on entry
							autoFocus
							value={title}
							onChange={(e) => setTitle(e.target.value)}
							onKeyDown={(e) => {
								if (e.key === "Enter") submitRename();
								if (e.key === "Escape") {
									setRenaming(false);
									setTitle(page.title);
								}
							}}
							onBlur={submitRename}
							className="h-6 w-full min-w-0 rounded border border-border/40 bg-background px-1.5 text-[13px] outline-none focus:border-primary/50"
						/>
					</div>
				) : (
					<div className="relative flex items-center">
						<SidebarMenuButton
							className="h-7 flex-1 pr-14 text-[13px]"
							style={{ paddingLeft: `${12 + depth * 14}px` }}
							onClick={() =>
								navigate({
									to: "/projects/$projectId/docs/wiki",
									params: { projectId },
									search: { page: page.url },
								})
							}
						>
							<FileText className="size-3.5 shrink-0 text-sidebar-foreground/50" />
							<span className="truncate">
								{page.title || t("docs.untitled")}
							</span>
						</SidebarMenuButton>
						{canWrite && (
							<div className="absolute right-1.5 flex items-center gap-0.5 opacity-0 group-hover/wikipage:opacity-100 transition-opacity">
								<button
									type="button"
									title={t("docs.rename", { defaultValue: "Rename" })}
									onClick={(e) => {
										e.stopPropagation();
										setTitle(page.title);
										setRenaming(true);
									}}
									className="flex size-5 items-center justify-center rounded text-sidebar-foreground/45 hover:bg-sidebar-accent hover:text-sidebar-foreground"
								>
									<Pencil className="size-3" />
								</button>
								<button
									type="button"
									title={t("docs.delete", { defaultValue: "Delete" })}
									onClick={(e) => {
										e.stopPropagation();
										onRequestDelete(page);
									}}
									className="flex size-5 items-center justify-center rounded text-sidebar-foreground/45 transition-colors hover:bg-sidebar-accent hover:text-destructive"
								>
									<Trash2 className="size-3" />
								</button>
							</div>
						)}
					</div>
				)}
			</SidebarMenuItem>
			{(page.children ?? []).map((child) => (
				<WikiPageNode
					key={child.id}
					projectId={projectId}
					page={child}
					depth={depth + 1}
					canWrite={canWrite}
					onRequestDelete={onRequestDelete}
				/>
			))}
		</>
	);
}

/** Documentation section backed by the project's Wiki space (ADR-042). */
function WikiDocsSidebarSection({ projectId }: { projectId: string }) {
	const { t } = useTranslation("appShell");
	const qc = useQueryClient();
	const navigate = useNavigate();
	const { hasProjectPermission } = useProjectPermissions(projectId);
	const canWrite = hasProjectPermission("docs.write");
	const [collapsed, setCollapsed] = useState(false);
	const [deleteTarget, setDeleteTarget] = useState<WikiPage | null>(null);
	const [deleteOpen, setDeleteOpen] = useState(false);

	const { data: tree } = useQuery(wikiTreeQueryOptions(projectId));

	const newPageMutation = useMutation({
		mutationFn: () => createWikiPage(projectId, t("docs.untitled")),
		onSuccess: (created) => {
			qc.invalidateQueries({ queryKey: wikiQueryKeys.tree(projectId) });
			navigate({
				to: "/projects/$projectId/docs/wiki",
				params: { projectId },
				search: { page: created.url },
			});
		},
	});

	const { state: sidebarState } = useSidebar();
	if (sidebarState === "collapsed") {
		return (
			<SidebarGroup>
				<SidebarGroupContent>
					<SidebarMenu>
						<SidebarMenuItem>
							<SidebarMenuButton tooltip={t("docs.documentation")}>
								<BookOpen className="size-4" />
							</SidebarMenuButton>
						</SidebarMenuItem>
					</SidebarMenu>
				</SidebarGroupContent>
			</SidebarGroup>
		);
	}

	return (
		<SidebarGroup className="px-0">
			<SidebarGroupLabel
				className="flex cursor-pointer items-center justify-between hover:text-sidebar-foreground transition-colors px-3"
				onClick={() => setCollapsed((prev) => !prev)}
			>
				<span>{t("docs.documentation")}</span>
				<ChevronRight
					className={cn(
						"size-3.5 transition-transform duration-200 text-sidebar-foreground/40",
						!collapsed && "rotate-90",
					)}
				/>
			</SidebarGroupLabel>

			{!collapsed && (
				<SidebarGroupContent>
					<div className="py-1 space-y-0.5">
						<SidebarMenu>
							<SidebarMenuItem>
								<SidebarMenuButton
									className="h-7 text-[13px]"
									onClick={() =>
										navigate({
											to: "/projects/$projectId/docs/wiki",
											params: { projectId },
											search: {},
										})
									}
								>
									<BookOpen className="size-3.5 shrink-0 text-sidebar-foreground/50" />
									<span className="truncate">
										{t("docs.openSpace", { defaultValue: "Open space" })}
									</span>
								</SidebarMenuButton>
							</SidebarMenuItem>
							{(tree?.items ?? []).map((page) => (
								<WikiPageNode
									key={page.id}
									projectId={projectId}
									page={page}
									depth={0}
									canWrite={canWrite}
									onRequestDelete={(target) => {
										setDeleteTarget(target);
										setDeleteOpen(true);
									}}
								/>
							))}
							{canWrite && (
								<SidebarMenuItem>
									<SidebarMenuButton
										className="h-7 text-[13px] text-sidebar-foreground/60"
										onClick={() => newPageMutation.mutate()}
										disabled={newPageMutation.isPending}
									>
										<Plus className="size-3.5 shrink-0" />
										<span>{t("docs.newDocument")}</span>
									</SidebarMenuButton>
								</SidebarMenuItem>
							)}
						</SidebarMenu>
					</div>
				</SidebarGroupContent>
			)}
			<WikiPageDeleteDialog
				projectId={projectId}
				page={deleteTarget}
				open={deleteOpen}
				onOpenChange={setDeleteOpen}
			/>
		</SidebarGroup>
	);
}

// ── Project Switcher ───────────────────────────────────────────────────────────
function ProjectSwitcher({
	currentProjectId,
	canCreate,
}: {
	currentProjectId?: string;
	canCreate: boolean;
}) {
	const { t } = useTranslation("appShell");
	const [open, setOpen] = useState(false);
	const { data: projectsResult } = useQuery(projectsQueryOptions());
	const { data: currentProject } = useQuery({
		...projectQueryOptions(currentProjectId ?? ""),
		enabled: !!currentProjectId,
	});

	const projects = projectsResult?.items ?? [];
	const label = currentProject?.name ?? t("projectSwitcher.projects");
	const initials = currentProject?.name
		? currentProject.name.slice(0, 2).toUpperCase()
		: null;

	const { data: user } = useQuery(currentUserOptionalQueryOptions);

	if (!user) {
		return (
			<div className="flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-sm font-medium text-sidebar-foreground/80 select-none">
				<div className="flex size-5 shrink-0 items-center justify-center rounded-md bg-primary/15 text-primary text-xs font-bold">
					{initials ?? <FolderKanban className="size-3" />}
				</div>
				<span className="flex-1 truncate text-left">{label}</span>
			</div>
		);
	}

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			<DropdownMenuTrigger
				className={cn(
					"flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-sm font-medium text-sidebar-foreground/80 transition-all duration-150 select-none cursor-pointer",
					open
						? "bg-primary/10 text-primary"
						: "hover:bg-sidebar-accent/60 hover:text-sidebar-foreground",
				)}
			>
				<div className="flex size-5 shrink-0 items-center justify-center rounded-md bg-primary/15 text-primary text-xs font-bold">
					{initials ?? <FolderKanban className="size-3" />}
				</div>
				<span className="flex-1 truncate text-left">{label}</span>
				<ChevronDown
					className={cn(
						"size-3.5 shrink-0 opacity-40 transition-transform duration-200",
						open && "rotate-180",
					)}
				/>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="start" sideOffset={6} className="w-60">
				<DropdownMenuGroup>
					<DropdownMenuLabel className="text-xs text-muted-foreground pb-1">
						{t("projectSwitcher.yourProjects")}
					</DropdownMenuLabel>
				</DropdownMenuGroup>
				<DropdownMenuSeparator />
				{projects.length > 0 ? (
					<DropdownMenuGroup>
						{projects.map((p) => (
							<DropdownMenuItem
								key={p.id}
								render={
									<Link
										to="/projects/$projectId"
										params={{ projectId: p.id }}
										className="flex items-center gap-2"
									/>
								}
							>
								<div className="flex size-5 shrink-0 items-center justify-center rounded bg-primary/15 text-primary text-xs font-bold">
									{p.name.slice(0, 2).toUpperCase()}
								</div>
								<span className="truncate">{p.name}</span>
								{p.id === currentProjectId && (
									<span className="ml-auto size-1.5 rounded-full bg-primary" />
								)}
							</DropdownMenuItem>
						))}
					</DropdownMenuGroup>
				) : (
					<div className="flex flex-col items-center gap-1 px-3 py-4">
						<div className="flex size-8 items-center justify-center rounded-md bg-muted">
							<FolderKanban className="size-4 text-muted-foreground" />
						</div>
						<p className="text-xs text-muted-foreground mt-0.5">
							{t("projectSwitcher.noProjectsYet")}
						</p>
					</div>
				)}
				<DropdownMenuSeparator />
				{canCreate ? (
					<DropdownMenuItem render={<Link to="/home" />}>
						<Plus className="size-3.5" />
						{t("projectSwitcher.newProject")}
					</DropdownMenuItem>
				) : null}
			</DropdownMenuContent>
		</DropdownMenu>
	);
}

// ── Nav Item ───────────────────────────────────────────────────────────────────
function NavItem({
	to,
	icon: Icon,
	label,
	badge,
	exact,
}: {
	to: string;
	icon: ComponentType<{ className?: string }>;
	label: string;
	badge?: string;
	/** Match only the exact path, not any sub-route below it. Use this when
	 * a deeper route (e.g. a plugin page under this segment) has its own
	 * distinct nav item and shouldn't also light up this one. */
	exact?: boolean;
}) {
	const location = useRouterState({ select: (s) => s.location.pathname });
	const isActive = exact
		? location === to
		: location === to || location.startsWith(`${to}/`);

	return (
		<SidebarMenuItem>
			<SidebarMenuButton
				isActive={isActive}
				tooltip={label}
				render={<Link to={to} />}
				className={cn(
					"relative transition-all duration-150",
					isActive
						? "bg-primary/10 text-primary font-medium before:absolute before:left-0 before:inset-y-2 before:w-0.75 before:rounded-full before:bg-primary"
						: "hover:bg-sidebar-accent/60",
				)}
			>
				<Icon className="size-4" />
				<span>{label}</span>
				{badge ? (
					<Badge className="ml-auto text-xs" variant="secondary">
						{badge}
					</Badge>
				) : null}
			</SidebarMenuButton>
		</SidebarMenuItem>
	);
}

// ── Project Nav ───────────────────────────────────────────────────────────────
// The in-app "Agents" surface was retired (ADR-038): agents live in the
// platform ChatDock now, so it no longer appears in the project nav.
const PROJECT_NAV_ITEMS = [
	{ segment: "automation", icon: Workflow, labelKey: "nav.automation" },
	{ segment: "team", icon: Users, labelKey: "nav.team" },
	{ segment: "efficiency", icon: Gauge, labelKey: "nav.efficiency" },
	{ segment: "settings", icon: Settings, labelKey: "nav.settings" },
] as const;

// Project settings sections, mirroring the settings page's own `NAV_ITEMS`
// (same ids, icons and i18n label keys from the `projects` namespace). Rendered
// as sub-items under the collapsible "Settings" group so there is no in-content
// sub-rail. `requiresDelete` mirrors the page's `visibleNavItems` gating: the
// Danger Zone is only shown to users who can delete the project.
const SETTINGS_SECTIONS = [
	{
		id: "general",
		labelKey: "project.settingsPage.nav.general",
		icon: Settings,
	},
	{ id: "roles", labelKey: "project.settingsPage.nav.roles", icon: Shield },
	{
		id: "task-statuses",
		labelKey: "project.settingsPage.nav.taskStatuses",
		icon: LayoutList,
	},
	{
		id: "task-types",
		labelKey: "project.settingsPage.nav.taskTypes",
		icon: Tag,
	},
	{
		id: "custom-fields",
		labelKey: "project.settingsPage.nav.customFields",
		icon: Plus,
	},
	{
		id: "workflow",
		labelKey: "project.settingsPage.nav.workflow",
		icon: GitBranch,
	},
	{
		id: "versions",
		labelKey: "project.settingsPage.nav.versions",
		icon: Milestone,
	},
	{
		id: "components",
		labelKey: "project.settingsPage.nav.components",
		icon: Package,
	},
	{
		id: "danger",
		labelKey: "project.settingsPage.nav.dangerZone",
		icon: AlertTriangle,
		requiresDelete: true,
	},
] as const;

function ProjectNav() {
	const { t } = useTranslation("appShell");
	return (
		<SidebarGroup>
			<SidebarGroupContent>
				<SidebarMenu>
					<SidebarMenuItem>
						<SidebarMenuButton
							tooltip={t("nav.allProjects")}
							render={<Link to="/home" />}
							className="text-muted-foreground hover:text-foreground hover:bg-sidebar-accent/60 transition-all"
						>
							<ArrowLeft className="size-4" />
							<span>{t("nav.allProjects")}</span>
						</SidebarMenuButton>
					</SidebarMenuItem>
				</SidebarMenu>
			</SidebarGroupContent>
		</SidebarGroup>
	);
}

const ANON_HIDDEN_SEGMENTS = new Set(["automation", "team", "settings"]);

/**
 * Collapsible "Settings" group in the project sidebar. Mirrors the
 * `PluginNavGroup` pattern (Collapsible + SidebarMenuSub, chevron rotates on
 * `group-data-[open]`, auto-opens when a child route is active): the gear-icon
 * parent is a pure toggle and each settings section nests as a sub-item that
 * deep-links to `/projects/:id/settings?section=<id>`. Section visibility
 * mirrors the settings page — the Danger Zone only appears for users who can
 * delete the project (global `projects.delete` or the project-scoped role).
 */
function SettingsNavGroup({
	projectId,
	location,
}: {
	projectId: string;
	location: string;
}) {
	const { t } = useTranslation("appShell");
	const { t: tProjects } = useTranslation("projects");
	const { hasPermission } = usePermissions();
	const { hasProjectPermission } = useProjectPermissions(projectId);
	const currentSection = useRouterState({
		select: (s) =>
			(s.location.search as { section?: string }).section ?? "general",
	});

	const canDelete =
		hasPermission("projects.delete") || hasProjectPermission("projects.delete");
	const sections = SETTINGS_SECTIONS.filter(
		(section) =>
			!("requiresDelete" in section && section.requiresDelete) || canDelete,
	);

	const settingsPath = `/projects/${projectId}/settings`;
	const onSettings =
		location === settingsPath || location.startsWith(`${settingsPath}/`);

	const [open, setOpen] = useState(onSettings);
	// Auto-open when the user navigates into settings from elsewhere (the
	// sidebar persists across route changes, so initial state alone isn't enough).
	useEffect(() => {
		if (onSettings) setOpen(true);
	}, [onSettings]);

	return (
		<Collapsible open={open} onOpenChange={setOpen} className="group/setnav">
			<SidebarMenuItem>
				<CollapsibleTrigger
					render={
						<SidebarMenuButton
							tooltip={t("nav.settings")}
							className={cn(
								"transition-all duration-150",
								onSettings
									? "text-primary font-medium"
									: "hover:bg-sidebar-accent/60",
							)}
						/>
					}
				>
					<Settings className="size-4" />
					<span>{t("nav.settings")}</span>
					<ChevronRight className="ml-auto size-4 transition-transform duration-150 group-data-[open]/setnav:rotate-90" />
				</CollapsibleTrigger>
				<CollapsibleContent>
					<SidebarMenuSub>
						{sections.map(({ id, labelKey, icon: Icon }) => {
							const isActive = onSettings && currentSection === id;
							return (
								<SidebarMenuSubItem key={id}>
									<SidebarMenuSubButton
										isActive={isActive}
										render={
											<Link
												to="/projects/$projectId/settings"
												params={{ projectId }}
												search={{ section: id }}
											/>
										}
										className={cn(
											isActive ? "bg-primary/10 text-primary font-medium" : "",
										)}
									>
										<Icon className="size-4" />
										<span>{tProjects(labelKey)}</span>
									</SidebarMenuSubButton>
								</SidebarMenuSubItem>
							);
						})}
					</SidebarMenuSub>
				</CollapsibleContent>
			</SidebarMenuItem>
		</Collapsible>
	);
}

function ProjectNavItems({
	projectId,
	isAnonymous,
}: {
	projectId: string;
	isAnonymous?: boolean;
}) {
	const { t } = useTranslation("appShell");
	const location = useRouterState({ select: (s) => s.location.pathname });

	const [collapsed, setCollapsed] = useState(() => {
		try {
			return (
				localStorage.getItem(`paca:sidebar-project-collapsed:${projectId}`) ===
				"true"
			);
		} catch {
			return false;
		}
	});

	const toggle = () => {
		setCollapsed((prev) => {
			const next = !prev;
			try {
				localStorage.setItem(
					`paca:sidebar-project-collapsed:${projectId}`,
					String(next),
				);
			} catch {
				/* ignore */
			}
			return next;
		});
	};

	return (
		<SidebarGroup>
			<SidebarGroupLabel
				className="flex cursor-pointer items-center justify-between hover:text-sidebar-foreground transition-colors"
				onClick={toggle}
			>
				<span>{t("nav.project")}</span>
				<ChevronRight
					className={cn(
						"size-3.5 transition-transform duration-200 text-sidebar-foreground/40",
						!collapsed && "rotate-90",
					)}
				/>
			</SidebarGroupLabel>

			{!collapsed && (
				<SidebarGroupContent>
					<SidebarMenu>
						{PROJECT_NAV_ITEMS.filter(
							(item) =>
								// Settings is rendered as a collapsible group (below) instead
								// of a flat item, so its sections nest in the sidebar.
								item.segment !== "settings" &&
								(!isAnonymous || !ANON_HIDDEN_SEGMENTS.has(item.segment)),
						).map(({ segment, icon: Icon, labelKey }) => {
							const href = segment
								? `/projects/${projectId}/${segment}`
								: `/projects/${projectId}`;
							const isActive = segment
								? location.startsWith(href)
								: location === href || location === `${href}/`;
							const label = t(labelKey);
							return (
								<SidebarMenuItem key={labelKey}>
									<SidebarMenuButton
										isActive={isActive}
										tooltip={label}
										render={<Link to={href} />}
										className={cn(
											"relative transition-all duration-150",
											isActive
												? "bg-primary/10 text-primary font-medium before:absolute before:left-0 before:inset-y-2 before:w-0.75 before:rounded-full before:bg-primary"
												: "hover:bg-sidebar-accent/60",
										)}
									>
										<Icon className="size-4" />
										<span>{label}</span>
									</SidebarMenuButton>
								</SidebarMenuItem>
							);
						})}
						{(!isAnonymous || !ANON_HIDDEN_SEGMENTS.has("settings")) && (
							<SettingsNavGroup projectId={projectId} location={location} />
						)}
					</SidebarMenu>
				</SidebarGroupContent>
			)}
		</SidebarGroup>
	);
}

// ── Plugin-contributed pages ────────────────────────────────────────────────

/** Sidebar nav items for plugin `project.page` extension points (e.g. a
 * project-wide time-tracking view), routed to
 * /projects/:projectId/plugins/:pluginId/:slug. */
function pluginNavItemPath(projectId: string, item: PluginNavRegistration) {
	return `/projects/${projectId}/plugins/${item.pluginId}/${item.slug}`;
}

/** A single flat sidebar entry for a plugin that exposes exactly one page. */
function PluginNavLeaf({
	projectId,
	item,
	location,
}: {
	projectId: string;
	item: PluginNavRegistration;
	location: string;
}) {
	const Icon = resolvePluginIcon(item.icon);
	const to = pluginNavItemPath(projectId, item);
	const isActive = location === to || location.startsWith(`${to}/`);
	return (
		<SidebarMenuItem>
			<SidebarMenuButton
				isActive={isActive}
				tooltip={item.label}
				render={<Link to={to} />}
				className={cn(
					"relative transition-all duration-150",
					isActive
						? "bg-primary/10 text-primary font-medium before:absolute before:left-0 before:inset-y-2 before:w-0.75 before:rounded-full before:bg-primary"
						: "hover:bg-sidebar-accent/60",
				)}
			>
				<Icon className="size-4" />
				<span>{item.label}</span>
			</SidebarMenuButton>
		</SidebarMenuItem>
	);
}

/**
 * Collapsible sidebar group for a plugin that exposes MULTIPLE pages (e.g. the
 * native SDD Fleet plugin's eight views). The plugin's displayName becomes the
 * parent row; each nav item nests as a sub-page underneath, so there is no
 * second in-content sub-rail eating horizontal space (ADR-038). Owns its own
 * open/closed state and auto-expands whenever one of its children is active.
 */
function PluginNavGroup({
	projectId,
	items,
	location,
}: {
	projectId: string;
	items: PluginNavRegistration[];
	location: string;
}) {
	const head = items[0];
	const HeadIcon = resolvePluginIcon(head.icon);
	const childActive = items.some((it) => {
		const to = pluginNavItemPath(projectId, it);
		return location === to || location.startsWith(`${to}/`);
	});
	const [open, setOpen] = useState(childActive);
	// Auto-open when the user navigates into one of the group's pages from
	// elsewhere (the sidebar persists across route changes, so initial state
	// alone isn't enough).
	useEffect(() => {
		if (childActive) setOpen(true);
	}, [childActive]);

	return (
		<Collapsible open={open} onOpenChange={setOpen} className="group/plnav">
			<SidebarMenuItem>
				<CollapsibleTrigger
					render={
						<SidebarMenuButton
							tooltip={head.pluginName}
							className={cn(
								"transition-all duration-150",
								childActive
									? "text-primary font-medium"
									: "hover:bg-sidebar-accent/60",
							)}
						/>
					}
				>
					<HeadIcon className="size-4" />
					<span>{head.pluginName}</span>
					<ChevronRight className="ml-auto size-4 transition-transform duration-150 group-data-[open]/plnav:rotate-90" />
				</CollapsibleTrigger>
				<CollapsibleContent>
					<SidebarMenuSub>
						{items.map((item) => {
							const Icon = resolvePluginIcon(item.icon);
							const to = pluginNavItemPath(projectId, item);
							const isActive = location === to || location.startsWith(`${to}/`);
							return (
								<SidebarMenuSubItem key={`${item.pluginId}:${item.slug}`}>
									<SidebarMenuSubButton
										isActive={isActive}
										render={<Link to={to} />}
										className={cn(
											isActive ? "bg-primary/10 text-primary font-medium" : "",
										)}
									>
										<Icon className="size-4" />
										<span>{item.label}</span>
									</SidebarMenuSubButton>
								</SidebarMenuSubItem>
							);
						})}
					</SidebarMenuSub>
				</CollapsibleContent>
			</SidebarMenuItem>
		</Collapsible>
	);
}

function PluginProjectPages({ projectId }: { projectId: string }) {
	const { t } = useTranslation("appShell");
	const { getNavItems } = usePluginRegistry();
	const { hasProjectPermission } = useProjectPermissions(projectId);
	const location = useRouterState({ select: (s) => s.location.pathname });
	const navItems = getNavItems("project").filter(
		(item) =>
			!item.requiredPermission || hasProjectPermission(item.requiredPermission),
	);
	if (navItems.length === 0) return null;

	// Group items by plugin (insertion order preserved). A plugin that
	// contributes several pages is rendered as one collapsible parent so its
	// pages nest under the plugin name instead of flooding the flat list.
	const byPlugin = new Map<string, PluginNavRegistration[]>();
	for (const item of navItems) {
		const bucket = byPlugin.get(item.pluginId);
		if (bucket) bucket.push(item);
		else byPlugin.set(item.pluginId, [item]);
	}

	return (
		<SidebarGroup>
			<SidebarGroupLabel>{t("nav.plugins")}</SidebarGroupLabel>
			<SidebarGroupContent>
				<SidebarMenu>
					{[...byPlugin.entries()].map(([pluginId, items]) =>
						items.length === 1 ? (
							<PluginNavLeaf
								key={pluginId}
								projectId={projectId}
								item={items[0]}
								location={location}
							/>
						) : (
							<PluginNavGroup
								key={pluginId}
								projectId={projectId}
								items={items}
								location={location}
							/>
						),
					)}
				</SidebarMenu>
			</SidebarGroupContent>
		</SidebarGroup>
	);
}

/** Admin-sidebar nav items for plugin `admin.page` extension points (e.g. a
 * cross-project time-tracking summary), routed to
 * /admin/plugins/:pluginId/:slug. Rendered inline in the existing
 * "Administration" SidebarMenu, so no extra group wrapper here. `navItems`
 * is pre-filtered by the caller (each item's own `requiredPermission`, if
 * any) so this stays in sync with the `showAdminSection` computation that
 * decides whether the enclosing group renders at all. */
function PluginAdminPages({ navItems }: { navItems: PluginNavRegistration[] }) {
	return (
		<>
			{navItems.map((item) => {
				const Icon = resolvePluginIcon(item.icon);
				const to = `/admin/plugins/${item.pluginId}/${item.slug}`;
				return (
					<NavItem
						key={`${item.pluginId}:${item.slug}`}
						to={to}
						icon={Icon}
						label={item.label}
					/>
				);
			})}
		</>
	);
}

// ── Project Interactions Section ───────────────────────────────────────────────
function ProjectInteractionsSection({
	projectId,
	isAnonymous,
}: {
	projectId: string;
	isAnonymous?: boolean;
}) {
	const { t } = useTranslation("appShell");
	const location = useRouterState({ select: (s) => s.location.pathname });
	const { hasPermission } = usePermissions();
	const qc = useQueryClient();
	const [collapsed, setCollapsed] = useState(() => {
		try {
			return (
				localStorage.getItem(
					`paca:sidebar-interactions-collapsed:${projectId}`,
				) === "true"
			);
		} catch {
			return false;
		}
	});

	const { hasProjectPermission } = useProjectPermissions(projectId);

	// Check sprints.read via either the global role or the project role so that
	// users with a project-scoped "Editor" / "Viewer" role (global role = User)
	// can still see Timeline, Backlog, and open sprints.
	const canViewSprints =
		hasPermission("sprints.read") || hasProjectPermission("sprints.read");
	const canEditTasks =
		hasPermission("tasks.write") || hasProjectPermission("tasks.write");

	const [dragOverInteractionId, setDragOverInteractionId] = useState<
		string | null
	>(null);

	// Clear the drop-target highlight whenever any drag ends (covers drag-cancel
	// and mouse-release outside a valid target, where dragleave may not fire).
	useEffect(() => {
		const handleDragEnd = () => setDragOverInteractionId(null);
		document.addEventListener("dragend", handleDragEnd);
		return () => document.removeEventListener("dragend", handleDragEnd);
	}, []);

	const updateSprintMutation = useMutation({
		mutationFn: ({
			taskId,
			sprintId,
		}: {
			taskId: string;
			sprintId: string | null;
		}) => updateTask(projectId, taskId, { sprint_id: sprintId }),
		onSuccess: () => {
			qc.invalidateQueries({
				queryKey: ["projects", projectId, "tasks"],
			});
			qc.invalidateQueries({ queryKey: ["projects", projectId, "sprints"] });
		},
	});

	const handleInteractionDragOver = (
		e: React.DragEvent,
		interactionId: string,
	) => {
		if (!canEditTasks || isAnonymous) return;
		if (!e.dataTransfer.types.includes("application/x-paca-task-id")) return;
		e.preventDefault();
		e.dataTransfer.dropEffect = "move";
		setDragOverInteractionId(interactionId);
	};

	const handleInteractionDragLeave = (e: React.DragEvent) => {
		// Clear whenever leaving the item. If the cursor moves to a child element
		// within the same item, dragover immediately re-fires on the parent and
		// restores the highlight, so the brief gap is imperceptible.
		if (
			!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node | null)
		) {
			setDragOverInteractionId(null);
		}
	};

	const handleInteractionDrop = (
		e: React.DragEvent,
		sprintId: string | null,
	) => {
		e.preventDefault();
		setDragOverInteractionId(null);
		if (!canEditTasks) return;
		const taskId = e.dataTransfer.getData("text/plain");
		if (!taskId) return;
		updateSprintMutation.mutate({ taskId, sprintId });
	};

	const { data: sprints = [] } = useQuery({
		...sprintsQueryOptions(projectId),
		enabled: canViewSprints,
		retry: false,
		refetchInterval: 30_000,
	});

	// Hide entire section if user lacks the "View Sprints" permission
	// (anonymous visitors on public projects can always view interactions)
	if (!canViewSprints && !isAnonymous) return null;

	const openSprints = sprints
		.filter((s) => s.status === "active")
		.sort((a, b) => a.name.localeCompare(b.name));

	const backlogHref = `/projects/${projectId}/interactions/backlog`;
	const isBacklogActive = location.startsWith(backlogHref);

	const timelineHref = `/projects/${projectId}/interactions/timeline`;
	const isTimelineActive = location.startsWith(timelineHref);

	const toggle = () => {
		setCollapsed((prev) => {
			const next = !prev;
			try {
				localStorage.setItem(
					`paca:sidebar-interactions-collapsed:${projectId}`,
					String(next),
				);
			} catch {
				/* ignore */
			}
			return next;
		});
	};

	return (
		<SidebarGroup>
			<SidebarGroupLabel
				className="flex cursor-pointer items-center justify-between hover:text-sidebar-foreground transition-colors"
				onClick={toggle}
			>
				<span>{t("interactions.title")}</span>
				<ChevronRight
					className={cn(
						"size-3.5 transition-transform duration-200 text-sidebar-foreground/40",
						!collapsed && "rotate-90",
					)}
				/>
			</SidebarGroupLabel>

			{!collapsed && (
				<SidebarGroupContent>
					<SidebarMenu>
						{/* Timeline */}
						<SidebarMenuItem>
							<SidebarMenuButton
								isActive={isTimelineActive}
								tooltip={t("interactions.timeline")}
								render={<Link to={timelineHref} />}
								className={cn(
									"relative transition-all duration-150",
									isTimelineActive
										? "bg-primary/10 text-primary font-medium before:absolute before:left-0 before:inset-y-2 before:w-0.75 before:rounded-full before:bg-primary"
										: "hover:bg-sidebar-accent/60",
								)}
							>
								<GanttChart className="size-4" />
								<span>{t("interactions.timeline")}</span>
							</SidebarMenuButton>
						</SidebarMenuItem>
						{/* Product Backlog — always shown */}
						<SidebarMenuItem
							onDragOver={(e) => handleInteractionDragOver(e, "backlog")}
							onDragLeave={handleInteractionDragLeave}
							onDrop={(e) => handleInteractionDrop(e, null)}
						>
							<SidebarMenuButton
								isActive={isBacklogActive}
								tooltip={t("interactions.productBacklog")}
								render={<Link to={backlogHref} />}
								className={cn(
									"relative transition-all duration-150",
									isBacklogActive
										? "bg-primary/10 text-primary font-medium before:absolute before:left-0 before:inset-y-2 before:w-0.75 before:rounded-full before:bg-primary"
										: "hover:bg-sidebar-accent/60",
									dragOverInteractionId === "backlog" &&
										"ring-2 ring-primary/40 bg-primary/5 text-primary",
								)}
							>
								<BookOpen className="size-4" />
								<span>{t("interactions.productBacklog")}</span>
							</SidebarMenuButton>
						</SidebarMenuItem>
						{/* Open sprints */}
						{openSprints.map((sprint) => {
							const sprintHref = `/projects/${projectId}/interactions/sprints/${sprint.id}`;
							const isActive = location.startsWith(sprintHref);
							return (
								<SidebarMenuItem
									key={sprint.id}
									onDragOver={(e) => handleInteractionDragOver(e, sprint.id)}
									onDragLeave={handleInteractionDragLeave}
									onDrop={(e) => handleInteractionDrop(e, sprint.id)}
								>
									<SidebarMenuButton
										isActive={isActive}
										tooltip={sprint.name}
										render={<Link to={sprintHref} />}
										className={cn(
											"relative transition-all duration-150",
											isActive
												? "bg-primary/10 text-primary font-medium before:absolute before:left-0 before:inset-y-2 before:w-0.75 before:rounded-full before:bg-primary"
												: "hover:bg-sidebar-accent/60",
											dragOverInteractionId === sprint.id &&
												"ring-2 ring-primary/40 bg-primary/5 text-primary",
										)}
									>
										<KanbanSquare className="size-4" />
										<span className="flex-1 truncate">{sprint.name}</span>
									</SidebarMenuButton>
								</SidebarMenuItem>
							);
						})}
					</SidebarMenu>
				</SidebarGroupContent>
			)}
		</SidebarGroup>
	);
}

// ── Theme Switcher ─────────────────────────────────────────────────────────────
const THEME_MODES = [
	{ mode: "light" as ThemeMode, Icon: Sun, labelKey: "theme.light" },
	{ mode: "dark" as ThemeMode, Icon: Moon, labelKey: "theme.dark" },
	{ mode: "auto" as ThemeMode, Icon: Monitor, labelKey: "theme.auto" },
] as const;

function ThemeSwitcher() {
	const { t } = useTranslation("appShell");
	const { mode, set } = useThemeMode();
	const cycle = () =>
		set(mode === "light" ? "dark" : mode === "dark" ? "auto" : "light");
	const CurrentIcon = mode === "light" ? Sun : mode === "dark" ? Moon : Monitor;

	return (
		<>
			{/* Collapsed: single cycling icon button with tooltip */}
			<SidebarMenu className="hidden group-data-[collapsible=icon]:flex">
				<SidebarMenuItem>
					<SidebarMenuButton
						tooltip={t("theme.cycleTooltip", { mode })}
						onClick={cycle}
					>
						<CurrentIcon className="size-4" />
					</SidebarMenuButton>
				</SidebarMenuItem>
			</SidebarMenu>

			{/* Expanded: segmented 3-way control */}
			<div className="flex items-center justify-between px-2 py-1.5 group-data-[collapsible=icon]:hidden">
				<span className="text-xs font-medium text-sidebar-foreground/50 tracking-wide">
					{t("theme.label")}
				</span>
				<div className="flex items-center gap-0.5 rounded-md border border-sidebar-border bg-sidebar p-0.5">
					{THEME_MODES.map(({ mode: m, Icon, labelKey }) => (
						<button
							key={m}
							type="button"
							onClick={() => set(m)}
							title={t(labelKey)}
							className={cn(
								"flex size-6 items-center justify-center rounded transition-all duration-150",
								mode === m
									? "bg-sidebar-accent text-sidebar-accent-foreground shadow-sm"
									: "text-sidebar-foreground/40 hover:text-sidebar-foreground/70",
							)}
						>
							<Icon className="size-3.5" />
						</button>
					))}
				</div>
			</div>
		</>
	);
}

// ── App Sidebar ────────────────────────────────────────────────────────────────
export function AppSidebar() {
	const { t } = useTranslation("appShell");
	const { hasPermission } = usePermissions();
	const { resolvedMode } = useThemeMode();
	const { projectId } = useParams({ strict: false });
	const { data: user } = useQuery(currentUserOptionalQueryOptions);
	const { getNavItems } = usePluginRegistry();

	const canAccessGlobalRoles =
		hasPermission("global_roles.read") || hasPermission("global_roles.write");

	const canAccessUsers =
		hasPermission("users.read") || hasPermission("users.write");

	const canAccessPlugins = hasPermission("users.write");

	const canCreateProject = hasPermission("projects.create");

	// Plugin admin nav items are gated by their own declared
	// `requiredPermission` (falling back to open access if the plugin didn't
	// declare one), never by `canAccessPlugins` — a user shouldn't need
	// `users.write` just to reach a plugin page whose author scoped it to a
	// narrower, plugin-specific permission.
	const adminPluginNavItems = getNavItems("admin").filter(
		(item) =>
			!item.requiredPermission || hasPermission(item.requiredPermission),
	);

	const showAdminSection =
		canAccessGlobalRoles ||
		canAccessUsers ||
		canAccessPlugins ||
		adminPluginNavItems.length > 0;
	const isProjectContext = !!projectId;
	const isAnonymous = !user;

	return (
		<Sidebar collapsible="icon">
			{/* Brand */}
			<SidebarHeader className="gap-2 pb-2">
				<div className="flex items-center gap-2.5 px-2 pt-1">
					{user ? (
						<Link to="/home">
							<img
								src={
									resolvedMode === "dark"
										? "/paca-logo-dark.svg"
										: "/paca-logo.svg"
								}
								alt={t("brand.logoAlt")}
								className="size-8 shrink-0"
							/>
						</Link>
					) : (
						<img
							src={
								resolvedMode === "dark"
									? "/paca-logo-dark.svg"
									: "/paca-logo.svg"
							}
							alt={t("brand.logoAlt")}
							className="size-8 shrink-0"
						/>
					)}
					<span className="font-[Syne] font-bold text-base tracking-tight text-sidebar-foreground group-data-[collapsible=icon]:hidden">
						paca
					</span>
				</div>
				<div className="group-data-[collapsible=icon]:hidden">
					<ProjectSwitcher
						currentProjectId={projectId}
						canCreate={canCreateProject}
					/>
				</div>
			</SidebarHeader>

			<SidebarSeparator />

			{/* Navigation — switches between workspace and project context */}
			<SidebarContent>
				{isProjectContext ? (
					<>
						{user && <ProjectNav />}
						{user && <SidebarSeparator />}
						<ProjectInteractionsSection
							projectId={projectId}
							isAnonymous={isAnonymous}
						/>
						<SidebarSeparator />
						<DocsSectionSwitch projectId={projectId} />
						<SidebarSeparator />
						<ExtensionPoint
							point="sidebar.project.section"
							componentProps={{ projectId }}
						/>
						<SidebarSeparator />
						<PluginProjectPages projectId={projectId} />
						<ProjectNavItems projectId={projectId} isAnonymous={isAnonymous} />
					</>
				) : (
					<>
						{user && (
							<SidebarGroup>
								<SidebarGroupContent>
									<SidebarMenu>
										<NavItem to="/home" icon={Home} label={t("nav.home")} />
									</SidebarMenu>
								</SidebarGroupContent>
							</SidebarGroup>
						)}

						<ExtensionPoint point="sidebar.general.section" />
						{/* Admin section */}
						{showAdminSection ? (
							<>
								<SidebarSeparator />
								<SidebarGroup>
									<SidebarGroupLabel>
										{t("nav.administration")}
									</SidebarGroupLabel>
									<SidebarGroupContent>
										<SidebarMenu>
											{canAccessGlobalRoles ? (
												<NavItem
													to="/admin/global-roles"
													icon={Shield}
													label={t("nav.globalRoles")}
												/>
											) : null}
											{canAccessUsers ? (
												<NavItem
													to="/admin/users"
													icon={Users}
													label={t("nav.users")}
												/>
											) : null}
											{canAccessPlugins ? (
												<NavItem
													to="/admin/plugins"
													icon={Puzzle}
													label={t("nav.plugins")}
													exact
												/>
											) : null}
											<PluginAdminPages navItems={adminPluginNavItems} />
										</SidebarMenu>
									</SidebarGroupContent>
								</SidebarGroup>
							</>
						) : null}
					</>
				)}
			</SidebarContent>

			{/* Footer: theme toggle + user menu (language selector lives in the user menu) */}
			<SidebarSeparator />
			<SidebarFooter className="gap-1 pb-3">
				<ThemeSwitcher />
				<UserMenu />
			</SidebarFooter>

			<SidebarRail />
		</Sidebar>
	);
}
