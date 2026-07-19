// ADR-042 — Documentation tab backed by the embedded Galaxy AI Wiki.
// The iframe shows the project's Wiki space (or a specific page via the
// `page` search param); the toolbar provides in-space search and page
// creation through Paca's server-side proxy.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { FilePlus2, Loader2, Search } from "lucide-react";
import { useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
	createWikiPage,
	searchWiki,
	type WikiPage,
	wikiQueryKeys,
	wikiSpaceQueryOptions,
} from "@/lib/wiki-api";

export interface WikiDocSearch {
	page?: string;
}

export const Route = createFileRoute(
	"/_authenticated/projects/$projectId/docs/wiki",
)({
	validateSearch: (search: Record<string, unknown>): WikiDocSearch => ({
		page: typeof search.page === "string" ? search.page : undefined,
	}),
	component: WikiDocsPage,
});

function WikiDocsPage() {
	const { projectId } = Route.useParams();
	const { page } = Route.useSearch();
	const navigate = useNavigate();
	const qc = useQueryClient();
	const { t } = useTranslation("appShell");

	const {
		data: space,
		isPending,
		isError,
	} = useQuery(wikiSpaceQueryOptions(projectId));

	const [query, setQuery] = useState("");
	const [results, setResults] = useState<WikiPage[] | null>(null);
	const [searching, setSearching] = useState(false);
	const searchSeq = useRef(0);

	const iframeSrc = useMemo(() => {
		if (page) return page;
		return space?.url ?? "";
	}, [page, space?.url]);

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

	const newPageMutation = useMutation({
		mutationFn: () =>
			createWikiPage(
				projectId,
				t("docs.untitled", { defaultValue: "Untitled" }),
			),
		onSuccess: (created) => {
			qc.invalidateQueries({ queryKey: wikiQueryKeys.tree(projectId) });
			navigate({
				to: "/projects/$projectId/docs/wiki",
				params: { projectId },
				search: { page: created.url },
			});
		},
	});

	if (isPending) {
		return (
			<div className="flex h-full items-center justify-center">
				<Loader2 className="size-5 animate-spin text-muted-foreground" />
			</div>
		);
	}

	if (isError || !space) {
		return (
			<div className="flex h-full items-center justify-center text-sm text-muted-foreground">
				{t("docs.wikiUnavailable", {
					defaultValue: "The Wiki documentation service is not available.",
				})}
			</div>
		);
	}

	return (
		<div className="flex h-full flex-col">
			{/* Toolbar */}
			<div className="flex items-center gap-2 border-b px-3 py-2">
				<div className="relative flex-1 max-w-sm">
					<Search className="absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
					<Input
						value={query}
						onChange={(e) => setQuery(e.target.value)}
						onKeyDown={(e) => {
							if (e.key === "Enter") void runSearch();
							if (e.key === "Escape") {
								setQuery("");
								setResults(null);
							}
						}}
						placeholder={t("docs.searchWiki", {
							defaultValue: "Search documentation…",
						})}
						className="h-8 pl-7"
					/>
				</div>
				<Button
					size="sm"
					variant="outline"
					onClick={() => newPageMutation.mutate()}
					disabled={newPageMutation.isPending}
				>
					<FilePlus2 className="size-4" />
					{t("docs.newDocument", { defaultValue: "New Document" })}
				</Button>
			</div>

			{/* Search results overlay */}
			{results !== null && (
				<div className="border-b bg-muted/30 px-3 py-2 text-sm">
					{searching ? (
						<Loader2 className="size-4 animate-spin text-muted-foreground" />
					) : results.length === 0 ? (
						<span className="text-muted-foreground">
							{t("docs.noResults", { defaultValue: "No results" })}
						</span>
					) : (
						<ul className="space-y-1">
							{results.slice(0, 8).map((hit) => (
								<li key={hit.id}>
									<button
										type="button"
										className="text-left hover:underline"
										onClick={() => {
											setResults(null);
											navigate({
												to: "/projects/$projectId/docs/wiki",
												params: { projectId },
												search: { page: hit.url },
											});
										}}
									>
										{hit.title ||
											t("docs.untitled", { defaultValue: "Untitled" })}
									</button>
								</li>
							))}
						</ul>
					)}
				</div>
			)}

			{/* Embedded Wiki */}
			<iframe
				key={iframeSrc}
				src={iframeSrc}
				title={t("docs.documentation", { defaultValue: "Documentation" })}
				className="min-h-0 w-full flex-1 border-0"
				allow="clipboard-read; clipboard-write"
			/>
		</div>
	);
}
