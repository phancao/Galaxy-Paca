import { queryOptions } from "@tanstack/react-query";

import { apiClient } from "./api-client";
import type { SuccessEnvelope } from "./api-error";

// ── Shapes ────────────────────────────────────────────────────────────────────

export interface Project {
	id: string;
	name: string;
	description: string;
	is_public: boolean;
	task_id_prefix: string;
	settings: Record<string, unknown>;
	created_by?: string;
	created_at: string;
}

export interface ProjectListResult {
	items: Project[];
	total: number;
	page: number;
	page_size: number;
}

export interface ProjectMember {
	id: string;
	project_id: string;
	user_id: string;
	project_role_id: string;
	username: string;
	full_name: string;
	role_name: string;
	member_type?: string; // "human" | "agent"
	agent_id?: string;
	agent_name?: string;
	agent_handle?: string;
	// Galaxy (ADR-038): the member's user account is a non-human
	// service/bridge account — badge it in team lists.
	is_service?: boolean;
}

export interface ProjectRole {
	id: string;
	project_id?: string;
	role_name: string;
	permissions: Record<string, unknown>;
	created_at: string;
	updated_at: string;
}

// ── Project CRUD ──────────────────────────────────────────────────────────────

export async function listProjects(
	page = 1,
	pageSize = 50,
): Promise<ProjectListResult> {
	const { data } = await apiClient.instance.get<
		SuccessEnvelope<ProjectListResult>
	>("/projects", { params: { page, page_size: pageSize } });
	return data.data;
}

export async function getProject(projectId: string): Promise<Project> {
	const { data } = await apiClient.instance.get<SuccessEnvelope<Project>>(
		`/projects/${projectId}`,
	);
	return data.data;
}

export async function createProject(payload: {
	name: string;
	description?: string;
	task_id_prefix?: string;
	is_public?: boolean;
}): Promise<Project> {
	const { data } = await apiClient.instance.post<SuccessEnvelope<Project>>(
		"/projects",
		payload,
	);
	return data.data;
}

export async function updateProject(
	projectId: string,
	payload: {
		name?: string;
		description?: string;
		task_id_prefix?: string;
		is_public?: boolean;
	},
): Promise<Project> {
	const { data } = await apiClient.instance.patch<SuccessEnvelope<Project>>(
		`/projects/${projectId}`,
		payload,
	);
	return data.data;
}

export async function deleteProject(projectId: string): Promise<void> {
	await apiClient.instance.delete(`/projects/${projectId}`);
}

// ── Members ───────────────────────────────────────────────────────────────────

export async function listProjectMembers(
	projectId: string,
): Promise<ProjectMember[]> {
	const { data } = await apiClient.instance.get<
		SuccessEnvelope<ProjectMember[]>
	>(`/projects/${projectId}/members`);
	return data.data;
}

export async function addProjectMember(
	projectId: string,
	payload: { user_id: string; project_role_id: string },
): Promise<ProjectMember> {
	const { data } = await apiClient.instance.post<
		SuccessEnvelope<ProjectMember>
	>(`/projects/${projectId}/members`, payload);
	return data.data;
}

export async function updateProjectMemberRole(
	projectId: string,
	memberId: string,
	payload: { project_role_id: string },
): Promise<ProjectMember> {
	const { data } = await apiClient.instance.patch<
		SuccessEnvelope<ProjectMember>
	>(`/projects/${projectId}/members/${memberId}`, payload);
	return data.data;
}

export async function removeProjectMember(
	projectId: string,
	memberId: string,
): Promise<void> {
	await apiClient.instance.delete(`/projects/${projectId}/members/${memberId}`);
}

export async function getMyProjectPermissions(
	projectId: string,
): Promise<Record<string, boolean>> {
	const { data } = await apiClient.instance.get<
		SuccessEnvelope<{ permissions: Record<string, boolean> }>
	>(`/projects/${projectId}/members/me/permissions`);
	return data.data.permissions;
}

// ── Roles ─────────────────────────────────────────────────────────────────────

export async function listProjectRoles(
	projectId: string,
): Promise<ProjectRole[]> {
	const { data } = await apiClient.instance.get<SuccessEnvelope<ProjectRole[]>>(
		`/projects/${projectId}/roles`,
	);
	return data.data;
}

export async function createProjectRole(
	projectId: string,
	payload: { role_name: string; permissions?: Record<string, unknown> },
): Promise<ProjectRole> {
	const { data } = await apiClient.instance.post<SuccessEnvelope<ProjectRole>>(
		`/projects/${projectId}/roles`,
		payload,
	);
	return data.data;
}

export async function updateProjectRole(
	projectId: string,
	roleId: string,
	payload: { role_name: string; permissions?: Record<string, unknown> },
): Promise<ProjectRole> {
	const { data } = await apiClient.instance.patch<SuccessEnvelope<ProjectRole>>(
		`/projects/${projectId}/roles/${roleId}`,
		payload,
	);
	return data.data;
}

export async function deleteProjectRole(
	projectId: string,
	roleId: string,
): Promise<void> {
	await apiClient.instance.delete(`/projects/${projectId}/roles/${roleId}`);
}

// ── Task Types ────────────────────────────────────────────────────────────────

export interface TaskType {
	id: string;
	project_id: string;
	name: string;
	icon?: string | null;
	color?: string | null;
	description?: string | null;
	is_default?: boolean;
	is_system?: boolean;
	created_at: string;
	updated_at: string;
}

export async function listTaskTypes(projectId: string): Promise<TaskType[]> {
	const { data } = await apiClient.instance.get<
		SuccessEnvelope<{ items: TaskType[] }>
	>(`/projects/${projectId}/task-types`);
	return data.data.items;
}

export async function createTaskType(
	projectId: string,
	payload: {
		name: string;
		icon?: string | null;
		color?: string | null;
		description?: string | null;
	},
): Promise<TaskType> {
	const { data } = await apiClient.instance.post<SuccessEnvelope<TaskType>>(
		`/projects/${projectId}/task-types`,
		payload,
	);
	return data.data;
}

export async function updateTaskType(
	projectId: string,
	typeId: string,
	payload: {
		name?: string;
		icon?: string | null;
		color?: string | null;
		description?: string | null;
	},
): Promise<TaskType> {
	const { data } = await apiClient.instance.patch<SuccessEnvelope<TaskType>>(
		`/projects/${projectId}/task-types/${typeId}`,
		payload,
	);
	return data.data;
}

export async function deleteTaskType(
	projectId: string,
	typeId: string,
): Promise<void> {
	await apiClient.instance.delete(
		`/projects/${projectId}/task-types/${typeId}`,
	);
}

export async function setDefaultTaskType(
	projectId: string,
	typeId: string,
): Promise<TaskType> {
	const { data } = await apiClient.instance.put<SuccessEnvelope<TaskType>>(
		`/projects/${projectId}/task-types/${typeId}/set-default`,
	);
	return data.data;
}

// ── Task type role helpers ─────────────────────────────────────────────────────

/** Returns true if this task type is the system "Epic" type. */
export function isEpicType(t: TaskType | undefined | null): boolean {
	return !!t && !!t.is_system && t.name === "Epic";
}

/** Finds the Epic system type from a list of task types. */
export function findEpicType(types: TaskType[]): TaskType | undefined {
	return types.find(isEpicType);
}

/** Returns non-epic task types (Task, Bug, Story, etc). */
export function getNormalTaskTypes(types: TaskType[]): TaskType[] {
	return types.filter((t) => !isEpicType(t));
}

// ── Task Statuses ─────────────────────────────────────────────────────────────

export type StatusCategory =
	| "backlog"
	| "refinement"
	| "ready"
	| "todo"
	| "inprogress"
	| "done";

export const STATUS_CATEGORIES: StatusCategory[] = [
	"backlog",
	"refinement",
	"ready",
	"todo",
	"inprogress",
	"done",
];

export const STATUS_CATEGORY_LABELS: Record<StatusCategory, string> = {
	backlog: "Backlog",
	refinement: "Refinement",
	ready: "Ready",
	todo: "To Do",
	inprogress: "In Progress",
	done: "Done",
};

export interface TaskStatus {
	id: string;
	project_id: string;
	name: string;
	color?: string | null;
	position: number;
	category: StatusCategory;
	is_default?: boolean;
	created_at: string;
	updated_at: string;
}

export async function listTaskStatuses(
	projectId: string,
): Promise<TaskStatus[]> {
	const { data } = await apiClient.instance.get<
		SuccessEnvelope<{ items: TaskStatus[] }>
	>(`/projects/${projectId}/task-statuses`);
	return data.data.items;
}

export async function createTaskStatus(
	projectId: string,
	payload: {
		name: string;
		color?: string | null;
		position: number;
		category: StatusCategory;
	},
): Promise<TaskStatus> {
	const { data } = await apiClient.instance.post<SuccessEnvelope<TaskStatus>>(
		`/projects/${projectId}/task-statuses`,
		payload,
	);
	return data.data;
}

export async function updateTaskStatus(
	projectId: string,
	statusId: string,
	payload: {
		name?: string;
		color?: string | null;
		position?: number;
		category?: StatusCategory;
	},
): Promise<TaskStatus> {
	const { data } = await apiClient.instance.patch<SuccessEnvelope<TaskStatus>>(
		`/projects/${projectId}/task-statuses/${statusId}`,
		payload,
	);
	return data.data;
}

export async function deleteTaskStatus(
	projectId: string,
	statusId: string,
): Promise<void> {
	await apiClient.instance.delete(
		`/projects/${projectId}/task-statuses/${statusId}`,
	);
}

export async function setDefaultTaskStatus(
	projectId: string,
	statusId: string,
): Promise<TaskStatus> {
	const { data } = await apiClient.instance.put<SuccessEnvelope<TaskStatus>>(
		`/projects/${projectId}/task-statuses/${statusId}/set-default`,
	);
	return data.data;
}

/** Persists a new display order for task statuses in a single atomic request. */
export async function reorderTaskStatuses(
	projectId: string,
	orderedStatusIds: string[],
): Promise<void> {
	await apiClient.instance.put(
		`/projects/${projectId}/task-statuses/positions`,
		{
			status_ids: orderedStatusIds,
		},
	);
}

// ── Custom Field Definitions ─────────────────────────────────────────────────

export type FieldType =
	| "text"
	| "number"
	| "date"
	| "select"
	| "multi_select"
	| "boolean"
	| "url"
	// ADR-040 Phase 1: a "user" field stores a project member's UUID string; a
	// "label" field stores a free-form array of strings (open vocabulary tags).
	| "user"
	| "label";

// Permissive value type for a custom field's default (ADR-040 Phase 0).
// Shape depends on field_type: string (text/url/date/select), number, boolean,
// or string[] (multi_select). `null` means "no default".
export type CustomFieldDefaultValue =
	| string
	| number
	| boolean
	| string[]
	| null;

export interface CustomFieldDefinition {
	id: string;
	project_id: string;
	field_key: string;
	display_name: string;
	field_type: FieldType;
	options: string[];
	is_required: boolean;
	default_value: CustomFieldDefaultValue;
	// ADR-040 Phase 2.8: scopes a field to a single task type.
	// null → applies to all task types; a type id → only that type.
	// Set at creation, immutable on edit.
	task_type_id: string | null;
	created_at: string;
	updated_at: string;
}

export async function listCustomFieldDefinitions(
	projectId: string,
): Promise<CustomFieldDefinition[]> {
	const { data } = await apiClient.instance.get<
		SuccessEnvelope<{ items: CustomFieldDefinition[] }>
	>(`/projects/${projectId}/custom-fields`);
	return data.data.items;
}

export async function getCustomFieldDefinition(
	projectId: string,
	fieldId: string,
): Promise<CustomFieldDefinition> {
	const { data } = await apiClient.instance.get<
		SuccessEnvelope<CustomFieldDefinition>
	>(`/projects/${projectId}/custom-fields/${fieldId}`);
	return data.data;
}

export async function createCustomFieldDefinition(
	projectId: string,
	payload: {
		display_name: string;
		field_key: string;
		field_type: FieldType;
		options?: string[];
		is_required?: boolean;
		default_value?: CustomFieldDefaultValue;
		// ADR-040 Phase 2.8: null / omitted → all task types; a type id → scoped.
		task_type_id?: string | null;
	},
): Promise<CustomFieldDefinition> {
	const { data } = await apiClient.instance.post<
		SuccessEnvelope<CustomFieldDefinition>
	>(`/projects/${projectId}/custom-fields`, payload);
	return data.data;
}

export async function updateCustomFieldDefinition(
	projectId: string,
	fieldId: string,
	payload: {
		display_name?: string;
		options?: string[];
		is_required?: boolean;
		default_value?: CustomFieldDefaultValue;
	},
): Promise<CustomFieldDefinition> {
	const { data } = await apiClient.instance.patch<
		SuccessEnvelope<CustomFieldDefinition>
	>(`/projects/${projectId}/custom-fields/${fieldId}`, payload);
	return data.data;
}

export async function deleteCustomFieldDefinition(
	projectId: string,
	fieldId: string,
): Promise<void> {
	await apiClient.instance.delete(
		`/projects/${projectId}/custom-fields/${fieldId}`,
	);
}

// ── Workflow Status Transitions (ADR-040 Phase 1) ────────────────────────────

/**
 * A single allowed status transition rule for a project.
 *
 * - `task_type_id` null   → applies to every task type.
 * - `from_status_id` null → allowed from ANY source status.
 * - `required_fields`     → custom-field keys that must be set for the move.
 *
 * A project with ZERO transition rows allows free status movement; enforcement
 * only kicks in once at least one rule exists.
 */
export interface StatusTransition {
	id: string;
	project_id: string;
	task_type_id: string | null;
	from_status_id: string | null;
	to_status_id: string;
	required_fields: string[];
	created_at: string;
}

export async function listStatusTransitions(
	projectId: string,
): Promise<StatusTransition[]> {
	const { data } = await apiClient.instance.get<
		SuccessEnvelope<StatusTransition[]>
	>(`/projects/${projectId}/status-transitions`);
	return data.data;
}

export async function createStatusTransition(
	projectId: string,
	payload: {
		task_type_id?: string | null;
		from_status_id?: string | null;
		to_status_id: string;
		required_fields?: string[];
	},
): Promise<StatusTransition> {
	const { data } = await apiClient.instance.post<
		SuccessEnvelope<StatusTransition>
	>(`/projects/${projectId}/status-transitions`, payload);
	return data.data;
}

export async function deleteStatusTransition(
	projectId: string,
	transitionId: string,
): Promise<void> {
	await apiClient.instance.delete(
		`/projects/${projectId}/status-transitions/${transitionId}`,
	);
}

// ── Versions (ADR-040 Phase 2.9) ──────────────────────────────────────────────

/** A release/fix version a task can be assigned to via `version_id`. */
export interface ProjectVersion {
	id: string;
	project_id: string;
	name: string;
	description: string;
	released: boolean;
	release_date: string | null;
	archived: boolean;
	created_at: string;
	updated_at: string;
}

export async function listProjectVersions(
	projectId: string,
): Promise<ProjectVersion[]> {
	const { data } = await apiClient.instance.get<
		SuccessEnvelope<{ items: ProjectVersion[] }>
	>(`/projects/${projectId}/versions`);
	return data.data.items;
}

export async function createProjectVersion(
	projectId: string,
	payload: {
		name: string;
		description?: string;
		released?: boolean;
		release_date?: string | null;
		archived?: boolean;
	},
): Promise<ProjectVersion> {
	const { data } = await apiClient.instance.post<
		SuccessEnvelope<ProjectVersion>
	>(`/projects/${projectId}/versions`, payload);
	return data.data;
}

export async function updateProjectVersion(
	projectId: string,
	versionId: string,
	payload: {
		name?: string;
		description?: string;
		released?: boolean;
		release_date?: string | null;
		archived?: boolean;
	},
): Promise<ProjectVersion> {
	const { data } = await apiClient.instance.patch<
		SuccessEnvelope<ProjectVersion>
	>(`/projects/${projectId}/versions/${versionId}`, payload);
	return data.data;
}

export async function deleteProjectVersion(
	projectId: string,
	versionId: string,
): Promise<void> {
	await apiClient.instance.delete(
		`/projects/${projectId}/versions/${versionId}`,
	);
}

// ── Components (ADR-040 Phase 2.9) ────────────────────────────────────────────

/** A functional area a task can be assigned to via `component_id`. */
export interface ProjectComponent {
	id: string;
	project_id: string;
	name: string;
	description: string;
	lead_member_id: string | null;
	created_at: string;
	updated_at: string;
}

export async function listProjectComponents(
	projectId: string,
): Promise<ProjectComponent[]> {
	const { data } = await apiClient.instance.get<
		SuccessEnvelope<{ items: ProjectComponent[] }>
	>(`/projects/${projectId}/components`);
	return data.data.items;
}

export async function createProjectComponent(
	projectId: string,
	payload: {
		name: string;
		description?: string;
		lead_member_id?: string | null;
	},
): Promise<ProjectComponent> {
	const { data } = await apiClient.instance.post<
		SuccessEnvelope<ProjectComponent>
	>(`/projects/${projectId}/components`, payload);
	return data.data;
}

export async function updateProjectComponent(
	projectId: string,
	componentId: string,
	payload: {
		name?: string;
		description?: string;
		lead_member_id?: string | null;
	},
): Promise<ProjectComponent> {
	const { data } = await apiClient.instance.patch<
		SuccessEnvelope<ProjectComponent>
	>(`/projects/${projectId}/components/${componentId}`, payload);
	return data.data;
}

export async function deleteProjectComponent(
	projectId: string,
	componentId: string,
): Promise<void> {
	await apiClient.instance.delete(
		`/projects/${projectId}/components/${componentId}`,
	);
}

// ── Query Options ─────────────────────────────────────────────────────────────

export const projectsQueryOptions = (page = 1, pageSize = 50) =>
	queryOptions({
		queryKey: ["projects", { page, pageSize }],
		queryFn: () => listProjects(page, pageSize),
	});

export const projectQueryOptions = (projectId: string) =>
	queryOptions({
		queryKey: ["projects", projectId],
		queryFn: () => getProject(projectId),
		staleTime: 2 * 60 * 1000,
	});

export const projectMembersQueryOptions = (projectId: string) =>
	queryOptions({
		queryKey: ["projects", projectId, "members"],
		queryFn: () => listProjectMembers(projectId),
	});

export const myProjectPermissionsQueryOptions = (projectId: string) =>
	queryOptions({
		queryKey: ["projects", projectId, "members", "me", "permissions"],
		queryFn: () => getMyProjectPermissions(projectId),
		staleTime: 2 * 60 * 1000,
		retry: false,
	});

export const projectRolesQueryOptions = (projectId: string) =>
	queryOptions({
		queryKey: ["projects", projectId, "roles"],
		queryFn: () => listProjectRoles(projectId),
	});

export const taskTypesQueryOptions = (projectId: string) =>
	queryOptions({
		queryKey: ["projects", projectId, "task-types"],
		queryFn: () => listTaskTypes(projectId),
	});

export const taskStatusesQueryOptions = (projectId: string) =>
	queryOptions({
		queryKey: ["projects", projectId, "task-statuses"],
		queryFn: () => listTaskStatuses(projectId),
	});

export const customFieldsQueryOptions = (projectId: string) =>
	queryOptions({
		queryKey: ["projects", projectId, "custom-fields"],
		queryFn: () => listCustomFieldDefinitions(projectId),
	});

export const statusTransitionsQueryOptions = (projectId: string) =>
	queryOptions({
		queryKey: ["projects", projectId, "status-transitions"],
		queryFn: () => listStatusTransitions(projectId),
	});

export const projectVersionsQueryOptions = (projectId: string) =>
	queryOptions({
		queryKey: ["projects", projectId, "versions"],
		queryFn: () => listProjectVersions(projectId),
	});

export const projectComponentsQueryOptions = (projectId: string) =>
	queryOptions({
		queryKey: ["projects", projectId, "components"],
		queryFn: () => listProjectComponents(projectId),
	});
