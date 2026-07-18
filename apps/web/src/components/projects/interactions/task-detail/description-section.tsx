import "@blocknote/core/fonts/inter.css";
import "@blocknote/shadcn/style.css";

import { SideMenuController, useCreateBlockNote } from "@blocknote/react";
import { BlockNoteView } from "@blocknote/shadcn";
import { useMutation } from "@tanstack/react-query";
import { Sparkles } from "lucide-react";
import { useCallback, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { CustomSideMenu } from "@/components/shared/blocknote-custom-side-menu";
import { customSchema } from "@/components/shared/blocknote-schema";
import { normalizeBlockContent } from "@/components/shared/comment-blocknote";
import { MentionSuggestionMenus } from "@/components/shared/mention-suggestion-menus";
import { useThemeMode } from "@/hooks/use-theme-mode";
import { writeTaskDescriptionWithAI } from "@/lib/agent-api";
import {
	getAttachmentDownloadURL,
	uploadAttachment,
} from "@/lib/attachment-api";
import { useMentionData } from "@/lib/mention-api";
import { cleanBlocks, cn } from "@/lib/utils";

type UpdateFn = (payload: { description?: unknown[] | null }) => void;

interface DescriptionSectionProps {
	description?: unknown[] | null;
	canEdit?: boolean;
	projectId?: string;
	taskId?: string;
	/** Task title — sent as context to the one-shot "write with AI" call. */
	title?: string;
	onUpdate?: UpdateFn;
}

/** Custom URI scheme used to store attachment references in the markdown. */
const ATTACHMENT_SCHEME = "attachment://";

export function DescriptionSection({
	description,
	canEdit = true,
	projectId,
	taskId,
	title,
	onUpdate,
}: DescriptionSectionProps) {
	const { t } = useTranslation("projects");
	const { resolvedMode } = useThemeMode();
	const { teamMembers, tasks, documents } = useMentionData(projectId);

	// One-shot "write with AI" (ADR-038): the in-app agent runtime is retired
	// (agents live in the platform ChatDock now). Instead of picking a project
	// agent, we send the task's title + current description text to the API,
	// which calls the platform AI proxy and returns Markdown. We parse that into
	// BlockNote blocks, replace the editor content, and save — all in place.
	const writeWithAIMutation = useMutation({
		mutationFn: async () => {
			if (!projectId || !taskId) throw new Error("missing context");
			const current = await editor.blocksToMarkdownLossy(editor.document);
			return writeTaskDescriptionWithAI(
				projectId,
				taskId,
				title ?? "",
				current,
			);
		},
		onSuccess: async ({ text }) => {
			const blocks = await editor.tryParseMarkdownToBlocks(text);
			if (blocks.length === 0) return;
			editor.replaceBlocks(
				editor.document,
				blocks as Parameters<typeof editor.replaceBlocks>[1],
			);
			// replaceBlocks fires onChange (→ pendingRef); set it explicitly too
			// so the immediate save() below always persists the AI content.
			pendingRef.current = true;
			save();
		},
	});

	// Tracks the last value we wrote to the API, to avoid redundant saves and
	// to skip external refetch updates that match what we already have.
	const lastSavedRef = useRef<string | null>(null);
	// Whether the editor has been initialized with the description content.
	const initializedRef = useRef(false);
	// Whether there are unsaved changes pending a blur-save.
	const pendingRef = useRef(false);
	// Whether the editor has finished initial content population.
	const readyRef = useRef(false);

	// Keep projectId / taskId in refs so the stable editor callbacks always
	// reference the latest prop values without recreating the editor.
	const projectIdRef = useRef(projectId);
	const taskIdRef = useRef(taskId);
	useEffect(() => {
		projectIdRef.current = projectId;
	}, [projectId]);
	useEffect(() => {
		taskIdRef.current = taskId;
	}, [taskId]);

	const editor = useCreateBlockNote({
		schema: customSchema,
		/**
		 * Called by BlockNote when the user inserts an image / file / video / audio.
		 * Uploads via the task attachment API and returns a stable custom URI.
		 */
		uploadFile: async (file: File) => {
			const pId = projectIdRef.current;
			const tId = taskIdRef.current;
			if (!pId || !tId) throw new Error("No project/task context for upload");
			const attachment = await uploadAttachment(pId, tId, file);
			// Store as a stable custom URI; presigned URL is fetched on-demand.
			return `${ATTACHMENT_SCHEME}${pId}/${tId}/${attachment.id}`;
		},

		/**
		 * Called by BlockNote whenever it needs to display a file URL.
		 * Converts our custom `attachment://` URI into a fresh presigned URL.
		 */
		resolveFileUrl: async (url: string) => {
			if (!url.startsWith(ATTACHMENT_SCHEME)) return url;
			// URI format: attachment://{projectId}/{taskId}/{attachmentId}
			const path = url.slice(ATTACHMENT_SCHEME.length);
			const [pId, tId, attachmentId] = path.split("/");
			if (!pId || !tId || !attachmentId) return url;
			return getAttachmentDownloadURL(pId, tId, attachmentId);
		},
	});

	// Populate the editor from BlockNote JSON whenever description changes
	// externally (initial load or server refetch that differs from what we saved).
	useEffect(() => {
		// Normalize whatever the API returned (including legacy/invalid content
		// that isn't a block array, e.g. a plain string) into a safe block array
		// so BlockNote never receives data it can't render.
		const displayBlocks = normalizeBlockContent(description ?? null);
		const cleanedNormalized = cleanBlocks(
			displayBlocks.length > 0 ? displayBlocks : null,
		);
		// Stringify for stable comparison (array identity changes on every response)
		const normalizedStr = cleanedNormalized
			? JSON.stringify(cleanedNormalized)
			: null;
		if (initializedRef.current && normalizedStr === lastSavedRef.current)
			return;
		initializedRef.current = true;
		lastSavedRef.current = normalizedStr;
		readyRef.current = false;

		editor.replaceBlocks(
			editor.document,
			displayBlocks as Parameters<typeof editor.replaceBlocks>[1],
		);
		queueMicrotask(() => {
			readyRef.current = true;
		});
	}, [description, editor]);

	const handleChange = useCallback(() => {
		if (!canEdit || !readyRef.current) return;
		// Track dirty state so blur can save without re-reading document
		pendingRef.current = true;
	}, [canEdit]);

	const save = useCallback(() => {
		if (!canEdit || !pendingRef.current) return;
		pendingRef.current = false;
		const blocks = editor.document;
		// Consider empty when there is only one empty paragraph block
		const isEmpty =
			blocks.length === 1 &&
			blocks[0].type === "paragraph" &&
			Array.isArray(blocks[0].content) &&
			blocks[0].content.length === 0;

		const value: unknown[] | null = isEmpty ? null : (blocks as unknown[]);
		const cleanedValue = cleanBlocks(value);
		const valueStr = cleanedValue ? JSON.stringify(cleanedValue) : null;
		if (valueStr !== lastSavedRef.current) {
			lastSavedRef.current = valueStr;
			onUpdate?.({ description: cleanedValue });
		}
	}, [canEdit, editor, onUpdate]);

	// Save when focus leaves the entire editor container (mirrors title onBlur).
	const handleBlur = useCallback(
		(e: React.FocusEvent<HTMLDivElement>) => {
			// relatedTarget is the element receiving focus next.
			// If it's still inside this container, it's an internal focus move — don't save.
			if (e.currentTarget.contains(e.relatedTarget as Node)) return;
			save();
		},
		[save],
	);

	return (
		<div className="space-y-3">
			<div className="flex items-center justify-between">
				<h3 className="text-xs font-semibold uppercase tracking-[0.08em] text-muted-foreground/70 flex items-center gap-2">
					<span>{t("taskDetail.description.title")}</span>
					<div className="flex-1 h-px bg-linear-to-r from-border/40 to-transparent" />
				</h3>
				{canEdit && (
					<button
						type="button"
						disabled={writeWithAIMutation.isPending}
						className="flex items-center gap-1.5 text-xs text-muted-foreground/60 hover:text-muted-foreground transition-colors duration-150 font-medium disabled:opacity-60 disabled:cursor-not-allowed"
						onClick={() => writeWithAIMutation.mutate()}
					>
						<Sparkles
							className={cn(
								"size-3",
								writeWithAIMutation.isPending && "animate-pulse",
							)}
						/>
						{writeWithAIMutation.isPending
							? t("taskDetail.description.writeWithAIDialog.starting")
							: t("taskDetail.description.writeWithAI")}
					</button>
				)}
			</div>

			{writeWithAIMutation.error && (
				<p className="text-xs text-destructive">
					{(writeWithAIMutation.error as Error).message}
				</p>
			)}

			{/* biome-ignore lint/a11y/noStaticElementInteractions: wrapper captures blur from BlockNote rich-text editor */}
			<div
				className="rounded-xl border border-border/25 bg-card/50 hover:border-border/50 transition-all duration-200 overflow-hidden [&_.bn-editor]:min-h-20 [&_.bn-editor]:py-3 [&_.bn-editor]:text-base [&_.bn-editor]:leading-relaxed"
				onBlur={handleBlur}
			>
				<BlockNoteView
					editor={editor}
					editable={canEdit}
					onChange={handleChange}
					theme={resolvedMode}
					className="bn-shadcn"
					sideMenu={false}
				>
					<SideMenuController sideMenu={CustomSideMenu} />
					{canEdit && (
						<MentionSuggestionMenus
							editor={editor}
							teamMembers={teamMembers}
							tasks={tasks}
							documents={documents}
						/>
					)}
				</BlockNoteView>
			</div>

		</div>
	);
}
