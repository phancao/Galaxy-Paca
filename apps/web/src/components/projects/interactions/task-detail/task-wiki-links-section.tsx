// ADR-042 — "Linked pages": wiki pages linked to a task. Renders only when
// the Wiki integration is live (probe query succeeds); adding a link searches
// the project's wiki space through Paca's server-side proxy.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { BookOpen, FileText, Loader2, Plus, Trash2 } from "lucide-react";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import {
	addTaskWikiLink,
	removeTaskWikiLink,
	searchWiki,
	taskWikiLinksQueryOptions,
	type WikiPage,
	wikiSpaceQueryOptions,
} from "@/lib/wiki-api";

interface TaskWikiLinksSectionProps {
	projectId: string;
	taskId: string;
	canEdit?: boolean;
}

export function TaskWikiLinksSection({
	projectId,
	taskId,
	canEdit = true,
}: TaskWikiLinksSectionProps) {
	const { t } = useTranslation("projects");
	const qc = useQueryClient();
	const navigate = useNavigate();

	const probe = useQuery(wikiSpaceQueryOptions(projectId));
	const { data: links = [] } = useQuery({
		...taskWikiLinksQueryOptions(projectId, taskId),
		enabled: probe.isSuccess,
	});

	const [adding, setAdding] = useState(false);
	const [query, setQuery] = useState("");
	const [results, setResults] = useState<WikiPage[] | null>(null);
	const [searching, setSearching] = useState(false);
	const searchSeq = useRef(0);

	const invalidate = () =>
		qc.invalidateQueries({
			queryKey: taskWikiLinksQueryOptions(projectId, taskId).queryKey,
		});

	const addMutation = useMutation({
		mutationFn: (recordId: string) =>
			addTaskWikiLink(projectId, taskId, recordId),
		onSuccess: () => {
			setAdding(false);
			setQuery("");
			setResults(null);
			invalidate();
		},
	});

	const removeMutation = useMutation({
		mutationFn: (recordId: string) =>
			removeTaskWikiLink(projectId, taskId, recordId),
		onSuccess: invalidate,
	});

	// The Wiki integration is off — the whole section disappears.
	if (!probe.isSuccess) return null;

	const runSearch = async () => {
		const q = query.trim();
		if (!q) {
			setResults(null);
			return;
		}
		const seq = ++searchSeq.current;
		setSearching(true);
		try {
			const hits = await searchWiki(projectId, q);
			if (seq === searchSeq.current) setResults(hits);
		} finally {
			if (seq === searchSeq.current) setSearching(false);
		}
	};

	const openPage = (url: string) => {
		navigate({
			to: "/projects/$projectId/docs/wiki",
			params: { projectId },
			search: { page: url },
		});
	};

	return (
		<div className="space-y-3">
			<div className="flex items-center justify-between">
				<h3 className="text-xs font-semibold uppercase tracking-[0.08em] text-muted-foreground/70 flex items-center gap-2">
					<span>
						{t("taskDetail.wikiLinks.title", {
							defaultValue: "Linked pages",
						})}
					</span>
					<div className="flex-1 h-px bg-linear-to-r from-border/40 to-transparent" />
				</h3>
				{canEdit && (
					<button
						type="button"
						onClick={() => setAdding((prev) => !prev)}
						className="flex items-center gap-1.5 rounded-lg bg-primary/8 text-primary/80 hover:bg-primary/15 hover:text-primary px-2.5 py-1.5 text-xs font-semibold transition-all duration-150"
					>
						<Plus className="size-3" />
						{t("taskDetail.wikiLinks.addButton", { defaultValue: "Link page" })}
					</button>
				)}
			</div>

			{adding && (
				<div className="space-y-2">
					<Input
						autoFocus
						value={query}
						onChange={(e) => setQuery(e.target.value)}
						onKeyDown={(e) => {
							if (e.key === "Enter") void runSearch();
							if (e.key === "Escape") {
								setAdding(false);
								setQuery("");
								setResults(null);
							}
						}}
						placeholder={t("taskDetail.wikiLinks.searchPlaceholder", {
							defaultValue: "Search documentation, Enter to search…",
						})}
						className="h-8"
					/>
					{searching && (
						<Loader2 className="size-4 animate-spin text-muted-foreground" />
					)}
					{results !== null && !searching && (
						<div className="rounded-xl border border-border/25 bg-card/50 divide-y divide-border/15 overflow-hidden">
							{results.length === 0 ? (
								<p className="px-4 py-2.5 text-sm italic text-muted-foreground/45">
									{t("docs.noResults", { defaultValue: "No results" })}
								</p>
							) : (
								results.slice(0, 6).map((hit) => (
									<button
										key={hit.id}
										type="button"
										disabled={addMutation.isPending}
										onClick={() => addMutation.mutate(hit.id)}
										className="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-muted/30 transition-colors"
									>
										<FileText className="size-3 text-muted-foreground/40 shrink-0" />
										<span className="flex-1 text-sm truncate">
											{hit.title ||
												t("docs.untitled", { defaultValue: "Untitled" })}
										</span>
									</button>
								))
							)}
						</div>
					)}
				</div>
			)}

			{links.length > 0 ? (
				<div className="rounded-xl border border-border/25 bg-card/50 divide-y divide-border/15 overflow-hidden">
					{links.map((link) => (
						<div
							key={link.id}
							className="flex items-center gap-3 px-4 py-2.5 group"
						>
							<BookOpen className="size-3 text-muted-foreground/40 shrink-0" />
							{/* biome-ignore lint/a11y/useSemanticElements: span for styling and truncation */}
							<span
								role="button"
								tabIndex={0}
								className="flex-1 text-sm text-foreground truncate cursor-pointer hover:text-primary transition-colors duration-100"
								onClick={() => openPage(link.url)}
								onKeyDown={(e) => {
									if (e.key === "Enter" || e.key === " ") {
										e.preventDefault();
										openPage(link.url);
									}
								}}
							>
								{link.title || t("docs.untitled", { defaultValue: "Untitled" })}
							</span>
							{canEdit && (
								<button
									type="button"
									onClick={() => removeMutation.mutate(link.record_id)}
									className="size-6 rounded flex items-center justify-center text-muted-foreground/50 hover:text-destructive hover:bg-destructive/10 opacity-0 group-hover:opacity-100 transition-all duration-100"
									title={t("taskDetail.wikiLinks.removeLink", {
										defaultValue: "Remove link",
									})}
								>
									<Trash2 className="size-3" />
								</button>
							)}
						</div>
					))}
				</div>
			) : (
				!adding && (
					<div className="flex items-center gap-3 px-1 py-3 text-muted-foreground/45">
						<BookOpen className="size-4 opacity-70" />
						<p className="text-sm italic">
							{t("taskDetail.wikiLinks.empty", {
								defaultValue: "No linked pages yet",
							})}
						</p>
					</div>
				)
			)}
		</div>
	);
}
