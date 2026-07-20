import { useQuery } from "@tanstack/react-query";
import { listAllTasks } from "./interaction-api";
import { listProjectMembers, type ProjectMember } from "./project-api";
import {
	type WikiPage,
	wikiSpaceQueryOptions,
	wikiTreeQueryOptions,
} from "./wiki-api";

export interface TeamMember {
	id: string;
	name: string;
	username: string;
	avatar?: string | null | undefined;
}

export interface MentionableTask {
	id: string;
	title: string;
	task_number: number;
	status?: string | null | undefined;
}

export interface MentionableDocument {
	id: string;
	title: string;
}

export interface MentionData {
	teamMembers: TeamMember[];
	tasks: MentionableTask[];
	documents: MentionableDocument[];
}

const teamMembersQueryOptions = (projectId: string) => ({
	queryKey: ["projects", projectId, "members"],
	queryFn: () => listProjectMembers(projectId),
	staleTime: 5 * 60 * 1000,
});

const tasksQueryOptions = (projectId: string) => ({
	queryKey: ["projects", projectId, "tasks", "mentions"],
	queryFn: () => listAllTasks(projectId, { pageSize: 500 }),
	staleTime: 2 * 60 * 1000,
});

/** Flattens a wiki page tree into mentionable {id, title} entries. */
function flattenWikiPages(pages: WikiPage[], out: MentionableDocument[] = []) {
	for (const page of pages) {
		out.push({ id: page.id, title: page.title });
		if (page.children?.length) flattenWikiPages(page.children, out);
	}
	return out;
}

export function useMentionData(projectId?: string | null) {
	const { data: members = [] } = useQuery({
		...teamMembersQueryOptions(projectId ?? ""),
		enabled: !!projectId,
	});

	const { data: tasksResult } = useQuery({
		...tasksQueryOptions(projectId ?? ""),
		enabled: !!projectId,
	});

	// ADR-042: doc mentions suggest the project's Wiki pages (the native doc
	// feature was removed in Stage 6).
	const wikiProbe = useQuery({
		...wikiSpaceQueryOptions(projectId ?? ""),
		enabled: !!projectId,
	});
	const wikiEnabled = wikiProbe.isSuccess;

	const { data: wikiTree } = useQuery({
		...wikiTreeQueryOptions(projectId ?? ""),
		enabled: !!projectId && wikiEnabled,
	});

	const teamMembers: TeamMember[] = members.map((member: ProjectMember) => ({
		id:
			member.member_type === "agent"
				? (member.agent_id ?? member.user_id)
				: member.user_id,
		name: member.full_name,
		username: member.username,
		avatar: member.full_name.slice(0, 2).toUpperCase() || undefined,
	}));

	const tasks: MentionableTask[] =
		tasksResult?.items.map((task) => ({
			id: task.id,
			title: task.title,
			task_number: task.task_number,
			status: task.status_id || undefined,
		})) ?? [];

	const mentionDocs: MentionableDocument[] = wikiEnabled
		? flattenWikiPages(wikiTree?.items ?? [])
		: [];

	return {
		teamMembers,
		tasks,
		documents: mentionDocs,
		isLoading: !projectId,
	};
}
