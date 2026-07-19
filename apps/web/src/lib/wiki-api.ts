// ADR-042 — Wiki-backed Documentation surface. The Galaxy AI Wiki owns all
// document content; these endpoints are Paca's server-side proxy (space
// provisioning, page tree, search, task links). When the integration is not
// configured the API answers 503 WIKI_UNAVAILABLE and the UI falls back to
// the native docs surface.
import { queryOptions } from "@tanstack/react-query";

import { apiClient } from "./api-client";
import type { SuccessEnvelope } from "./api-error";

// ── Shapes ────────────────────────────────────────────────────────────────────

export interface WikiSpace {
	folder_id: string;
	url: string;
	created: boolean;
}

export interface WikiPage {
	id: string;
	title: string;
	url: string;
	context?: string;
	children?: WikiPage[];
}

export interface WikiTreeResult {
	space: WikiSpace;
	items: WikiPage[];
}

export interface WikiLink {
	id: string;
	record_id: string;
	url: string;
	title: string;
}

// ── Calls ─────────────────────────────────────────────────────────────────────

export async function getWikiSpace(projectId: string): Promise<WikiSpace> {
	const res = await apiClient.instance.get<SuccessEnvelope<WikiSpace>>(
		`/projects/${projectId}/wiki-space`,
	);
	return res.data.data;
}

export async function getWikiTree(projectId: string): Promise<WikiTreeResult> {
	const res = await apiClient.instance.get<SuccessEnvelope<WikiTreeResult>>(
		`/projects/${projectId}/wiki-space/tree`,
	);
	return res.data.data;
}

export async function searchWiki(
	projectId: string,
	query: string,
): Promise<WikiPage[]> {
	const res = await apiClient.instance.get<
		SuccessEnvelope<{ items: WikiPage[] }>
	>(`/projects/${projectId}/wiki-space/search`, { params: { q: query } });
	return res.data.data.items;
}

export async function createWikiPage(
	projectId: string,
	title: string,
): Promise<WikiPage> {
	const res = await apiClient.instance.post<SuccessEnvelope<WikiPage>>(
		`/projects/${projectId}/wiki-space/pages`,
		{ title },
	);
	return res.data.data;
}

export async function listTaskWikiLinks(
	projectId: string,
	taskId: string,
): Promise<WikiLink[]> {
	const res = await apiClient.instance.get<
		SuccessEnvelope<{ items: WikiLink[] }>
	>(`/projects/${projectId}/tasks/${taskId}/wiki-links`);
	return res.data.data.items;
}

export async function addTaskWikiLink(
	projectId: string,
	taskId: string,
	recordId: string,
): Promise<WikiLink> {
	const res = await apiClient.instance.post<SuccessEnvelope<WikiLink>>(
		`/projects/${projectId}/tasks/${taskId}/wiki-links`,
		{ record_id: recordId },
	);
	return res.data.data;
}

export async function removeTaskWikiLink(
	projectId: string,
	taskId: string,
	recordId: string,
): Promise<void> {
	await apiClient.instance.delete(
		`/projects/${projectId}/tasks/${taskId}/wiki-links/${recordId}`,
	);
}

// ── Query options ─────────────────────────────────────────────────────────────

export const wikiQueryKeys = {
	all: (projectId: string) => ["projects", projectId, "wiki"] as const,
	space: (projectId: string) =>
		["projects", projectId, "wiki", "space"] as const,
	tree: (projectId: string) => ["projects", projectId, "wiki", "tree"] as const,
	taskLinks: (projectId: string, taskId: string) =>
		["projects", projectId, "wiki", "task-links", taskId] as const,
};

/**
 * Probe + space info. retry:false so a 503 (integration disabled) settles
 * fast and the UI can fall back to the native docs surface; the result is
 * cached for the session — flipping the integration on requires a reload.
 */
export const wikiSpaceQueryOptions = (projectId: string) =>
	queryOptions({
		queryKey: wikiQueryKeys.space(projectId),
		queryFn: () => getWikiSpace(projectId),
		retry: false,
		staleTime: Number.POSITIVE_INFINITY,
	});

export const wikiTreeQueryOptions = (projectId: string) =>
	queryOptions({
		queryKey: wikiQueryKeys.tree(projectId),
		queryFn: () => getWikiTree(projectId),
		retry: false,
		staleTime: 30_000,
	});

export const taskWikiLinksQueryOptions = (projectId: string, taskId: string) =>
	queryOptions({
		queryKey: wikiQueryKeys.taskLinks(projectId, taskId),
		queryFn: () => listTaskWikiLinks(projectId, taskId),
		retry: false,
	});
