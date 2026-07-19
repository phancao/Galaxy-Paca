import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
	AlertTriangle,
	GitBranch,
	LayoutList,
	Milestone,
	Package,
	Plus,
	Settings,
	Shield,
	Tag,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { ComponentsSettings } from "@/components/projects/settings/ComponentsSettings";
import { CustomFieldsSettings } from "@/components/projects/settings/CustomFieldsSettings";
import { DangerZone } from "@/components/projects/settings/DangerZone";
import { GeneralSettings } from "@/components/projects/settings/GeneralSettings";
import { RolesSettings } from "@/components/projects/settings/RolesSettings";
import { TaskStatusesSettings } from "@/components/projects/settings/TaskStatusesSettings";
import { TaskTypesSettings } from "@/components/projects/settings/TaskTypesSettings";
import { VersionsSettings } from "@/components/projects/settings/VersionsSettings";
import { WorkflowSettings } from "@/components/projects/settings/WorkflowSettings";
import { usePermissions } from "@/hooks/use-permissions";
import { currentUserQueryOptions } from "@/lib/auth-api";
import { RemoteComponent } from "@/lib/plugins/loader";
import { usePluginRegistry } from "@/lib/plugins/registry";
import {
	customFieldsQueryOptions,
	type ProjectMember,
	type ProjectRole,
	projectComponentsQueryOptions,
	projectMembersQueryOptions,
	projectQueryOptions,
	projectRolesQueryOptions,
	projectVersionsQueryOptions,
	statusTransitionsQueryOptions,
	taskStatusesQueryOptions,
	taskTypesQueryOptions,
} from "@/lib/project-api";

type SettingsSearch = {
	/**
	 * Which settings section is visible. A kebab-case section id (general,
	 * roles, task-statuses, task-types, custom-fields, workflow, versions,
	 * components, danger) or a `plugin:<pluginId>:<component>` tab. Absent means
	 * "general". Navigation is owned by the project sidebar's collapsible
	 * "Settings" group, which deep-links each section via this param.
	 */
	section?: string;
};

export const Route = createFileRoute(
	"/_authenticated/projects/$projectId/settings/",
)({
	validateSearch: (search: Record<string, unknown>): SettingsSearch => {
		const section = search.section;
		return typeof section === "string" ? { section } : {};
	},
	loader: async ({ context: { queryClient }, params: { projectId } }) => {
		await Promise.all([
			queryClient.ensureQueryData(projectQueryOptions(projectId)),
			queryClient.ensureQueryData(projectRolesQueryOptions(projectId)),
			queryClient.ensureQueryData(projectMembersQueryOptions(projectId)),
			queryClient.ensureQueryData(taskStatusesQueryOptions(projectId)),
			queryClient.ensureQueryData(taskTypesQueryOptions(projectId)),
			queryClient.ensureQueryData(customFieldsQueryOptions(projectId)),
			queryClient.ensureQueryData(statusTransitionsQueryOptions(projectId)),
			queryClient.ensureQueryData(projectVersionsQueryOptions(projectId)),
			queryClient.ensureQueryData(projectComponentsQueryOptions(projectId)),
		]);
	},
	component: SettingsPage,
});

// ── Settings Page ─────────────────────────────────────────────────────────────

const NAV_ITEMS = [
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
	},
] as const;

function SettingsPage() {
	const { t } = useTranslation("projects");
	const { projectId } = Route.useParams();
	const { data: project } = useQuery(projectQueryOptions(projectId));
	const { hasPermission } = usePermissions();
	const { data: currentUser } = useQuery(currentUserQueryOptions);
	const { data: members = [] } = useQuery(
		projectMembersQueryOptions(projectId),
	);
	const { data: roles = [] } = useQuery(projectRolesQueryOptions(projectId));

	const myMembership = (members as ProjectMember[]).find(
		(m) => m.user_id === currentUser?.id,
	);
	const myRole = (roles as ProjectRole[]).find(
		(r) => r.id === myMembership?.project_role_id,
	);
	const hasProjectDelete = Boolean(
		(myRole?.permissions as Record<string, boolean> | undefined)?.[
			"projects.delete"
		],
	);
	const hasProjectWrite = Boolean(
		(myRole?.permissions as Record<string, boolean> | undefined)?.[
			"projects.write"
		],
	);
	const hasProjectRolesWrite = Boolean(
		(myRole?.permissions as Record<string, boolean> | undefined)?.[
			"project.roles.write"
		],
	);
	const canDelete = hasPermission("projects.delete") || hasProjectDelete;
	const canEditProject = hasPermission("projects.write") || hasProjectWrite;
	const canManageRoles =
		hasPermission("project.roles.write") || hasProjectRolesWrite;
	const hasTasksWrite = Boolean(
		(myRole?.permissions as Record<string, boolean> | undefined)?.[
			"tasks.write"
		],
	);
	const canManageTasks = hasPermission("tasks.write") || hasTasksWrite;

	const { getRegistrations } = usePluginRegistry();
	const pluginTabs = getRegistrations("project.settings.tab").filter(
		(r) => !r.hidden,
	);

	const visibleNavItems = canDelete
		? NAV_ITEMS
		: NAV_ITEMS.filter((i) => i.id !== "danger");

	// Section navigation now lives in Paca's own left sidebar (the collapsible
	// "Settings" group). The active section is driven entirely by the URL search
	// param so each sidebar child can deep-link to it — there is no in-content
	// sub-rail anymore. Falls back to "general" for a missing/unknown/forbidden
	// section (e.g. ?section=danger without delete permission).
	const { section } = Route.useSearch();
	const pluginTabIds = new Set(
		pluginTabs.map((reg) => `plugin:${reg.pluginId}:${reg.component}`),
	);
	const knownSectionIds = new Set<string>(visibleNavItems.map((i) => i.id));
	const activeSection: string =
		section && (knownSectionIds.has(section) || pluginTabIds.has(section))
			? section
			: "general";

	return (
		<div className="flex flex-col min-h-0 flex-1">
			{/* Header */}
			<div className="relative overflow-hidden border-b border-border/50 shrink-0">
				<div
					className="pointer-events-none absolute inset-0 opacity-50"
					style={{
						backgroundImage:
							"radial-gradient(circle, color-mix(in oklch, var(--color-primary) 12%, transparent) 1px, transparent 1px)",
						backgroundSize: "20px 20px",
						maskImage:
							"radial-gradient(ellipse 70% 100% at 0% 0%, black 20%, transparent 70%)",
					}}
				/>
				<div className="relative px-6 py-7 max-w-6xl mx-auto w-full">
					<div className="flex items-center gap-2.5 mb-1">
						<Settings className="size-4 text-muted-foreground" />
						<h1 className="font-[Syne] text-2xl font-bold tracking-tight">
							{t("project.settingsPage.title")}
						</h1>
					</div>
					<p className="text-sm text-muted-foreground">
						{project?.name} · {t("project.settingsPage.subtitle")}
					</p>
				</div>
			</div>

			{/* Body — full-width single section; the project sidebar's collapsible
			    "Settings" group owns section navigation now. */}
			<div className="flex-1 overflow-y-auto">
				<div className="max-w-6xl mx-auto w-full px-6 py-8">
					{activeSection === "general" && (
						<GeneralSettings projectId={projectId} canEdit={canEditProject} />
					)}
					{activeSection === "roles" && (
						<RolesSettings
							projectId={projectId}
							canManageRoles={canManageRoles}
						/>
					)}
					{activeSection === "task-statuses" && (
						<TaskStatusesSettings
							projectId={projectId}
							canWrite={canManageTasks}
						/>
					)}
					{activeSection === "task-types" && (
						<TaskTypesSettings
							projectId={projectId}
							canWrite={canManageTasks}
						/>
					)}
					{activeSection === "custom-fields" && (
						<CustomFieldsSettings
							projectId={projectId}
							canWrite={canManageTasks}
						/>
					)}
					{activeSection === "workflow" && (
						<WorkflowSettings projectId={projectId} canWrite={canManageTasks} />
					)}
					{activeSection === "versions" && (
						<VersionsSettings projectId={projectId} canWrite={canManageTasks} />
					)}
					{activeSection === "components" && (
						<ComponentsSettings
							projectId={projectId}
							canWrite={canManageTasks}
						/>
					)}
					{activeSection === "danger" && canDelete && (
						<DangerZone projectId={projectId} />
					)}
					{/* Plugin settings tabs */}
					{pluginTabs.map((reg) =>
						activeSection === `plugin:${reg.pluginId}:${reg.component}` ? (
							<RemoteComponent
								key={`${reg.pluginId}:${reg.component}`}
								registration={reg}
								componentProps={{ projectId, canEdit: canEditProject }}
							/>
						) : null,
					)}
				</div>
			</div>
		</div>
	);
}
