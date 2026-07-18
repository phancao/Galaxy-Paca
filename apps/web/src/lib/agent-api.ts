import { apiClient } from "./api-client";
import type { SuccessEnvelope } from "./api-error";

// The in-app AI agent surface (project agents, conversations, chat sessions,
// the floating chat) was retired in ADR-038 — the agent surface is the platform
// ChatDock now. All that remains here is the one in-context AI touchpoint left
// in Paca: generating a task description. It is a stateless one-shot against the
// platform AI proxy (see services/api .../galaxyai), not an agent conversation.

/**
 * One-shot "write task description with AI" (ADR-038). The caller sends the
 * task's title + current description text; the API mints a short-lived act_as
 * token, calls the platform AI proxy, and returns AI-written Markdown for the
 * client to parse into editor blocks.
 */
export async function writeTaskDescriptionWithAI(
	projectId: string,
	taskId: string,
	title: string,
	description: string,
): Promise<{ text: string }> {
	const { data } = await apiClient.instance.post<SuccessEnvelope<{ text: string }>>(
		`/projects/${projectId}/tasks/${taskId}/write-with-ai`,
		{ title, description },
	);
	return data.data;
}
