import React from "react";
import { LS_VIEW, type ViewKey } from "./config";
import { Icon } from "./icons";
import { LANGS, detectLang, makeT, saveLang, type Lang } from "./i18n";
import { clearSddCache } from "./sdd-api";
import { ensureThemeInjected } from "./theme";
import { VIEWS } from "./views";

/**
 * "SDD Fleet" — the plugin's single exposed component (ADR-038). Registered at
 * `view` + `project.page`, and reached from EIGHT project nav items that nest
 * as sub-pages under "SDD Fleet" in Paca's own left sidebar. Each nav item
 * routes to a distinct `slug` (overview / tasks / sessions / activity /
 * analytics / coordination / sdd / fleet); the host forwards that slug in the
 * prop bag, and this component renders exactly ONE full-width view for it — no
 * in-content sub-rail (that used to be a second nav column eating horizontal
 * space), no host router nesting, no iframe. Each view fetches SAME-ORIGIN
 * /sdd-api/* (shared 60 s cache) and renders native cards/tables/timelines.
 *
 * Class component + classic JSX on purpose (see tsconfig.json): the host share
 * scope provides the "react" specifier but not "react/jsx-runtime", and class
 * components survive a federation fallback to the bundled React copy where
 * hooks would crash.
 *
 * The host forwards different prop bags per surface (project.page passes
 * {projectId, slug}; the `view` extension point / dock passes neither). SDD
 * telemetry is team-wide, so we ignore projectId; when no slug is supplied we
 * fall back to the last-viewed page (localStorage) or "overview".
 */

// View key -> i18n title/subtitle prefix ("coordination" uses "coord.*").
const TITLE_PREFIX: Record<ViewKey, string> = {
	overview: "overview",
	tasks: "tasks",
	sessions: "sessions",
	activity: "activity",
	analytics: "analytics",
	coordination: "coord",
	sdd: "sdd",
	fleet: "fleet",
};

interface SddProps {
	projectId?: string;
	/** Sub-page selector forwarded by the host route ($slug.tsx). One of the
	 *  eight ViewKeys; anything else falls back to the remembered/default view. */
	slug?: string;
	/** TEST-ONLY (smoke.mjs): initial view + language + seeded data, never set by the host. */
	__view?: ViewKey;
	__lang?: Lang;
	__testData?: unknown;
	[key: string]: unknown;
}

interface SddState {
	lang: Lang;
	refreshNonce: number;
}

function isViewKey(v: unknown): v is ViewKey {
	return typeof v === "string" && VIEWS.some((view) => view.key === v);
}

/**
 * Resolve which single view to render, in priority order: an explicit test
 * override, the host-supplied slug, the last-viewed page (localStorage), then
 * "overview". Derived from props on every render so navigating between the
 * sidebar sub-pages (same route, changing param) swaps the view immediately.
 */
function resolveView(props: SddProps): ViewKey {
	if (isViewKey(props.__view)) return props.__view;
	if (isViewKey(props.slug)) return props.slug;
	if (typeof localStorage !== "undefined") {
		const saved = localStorage.getItem(LS_VIEW);
		if (isViewKey(saved)) return saved;
	}
	return "overview";
}

export default class SddFleetView extends React.Component<SddProps, SddState> {
	constructor(props: SddProps) {
		super(props);
		ensureThemeInjected();
		this.state = {
			lang: props.__lang ?? detectLang(),
			refreshNonce: 0,
		};
	}

	componentDidMount() {
		this.rememberView();
	}

	componentDidUpdate(prev: SddProps) {
		if (prev.slug !== this.props.slug) this.rememberView();
	}

	/** Persist the current slug so the dock/`view` surface (no slug) reopens it. */
	private rememberView = () => {
		if (typeof localStorage === "undefined") return;
		if (isViewKey(this.props.slug)) localStorage.setItem(LS_VIEW, this.props.slug);
	};

	private setLang = (lang: Lang) => {
		this.setState({ lang });
		saveLang(lang);
	};

	private refresh = () => {
		clearSddCache();
		this.setState((s) => ({ refreshNonce: s.refreshNonce + 1 }));
	};

	render() {
		const t = makeT(this.state.lang);
		const viewKey = resolveView(this.props);
		const active = VIEWS.find((v) => v.key === viewKey) ?? VIEWS[0];
		const prefix = TITLE_PREFIX[active.key];
		const Active = active.Component;

		return (
			<div className="gxsd-root">
				{/* Full-width single page — the sidebar owns navigation now, so there
				    is no in-content sub-rail. */}
				<div className="gxsd-main">
					<header className="gxsd-head">
						<div>
							<h2 className="gxsd-title">
								<span className="gxsd-rail-ico">
									<Icon name={active.icon} size={17} />
								</span>
								{t(`${prefix}.title`)}
							</h2>
							<p className="gxsd-sub">{t(`${prefix}.sub`)}</p>
						</div>
						<span className="gxsd-spacer" />
						<div className="gxsd-langs" role="group" aria-label="language">
							{LANGS.map((l) => (
								<button
									type="button"
									key={l.code}
									className={`gxsd-lang ${l.code === this.state.lang ? "active" : ""}`}
									onClick={() => this.setLang(l.code)}
								>
									{l.label}
								</button>
							))}
						</div>
						<button type="button" className="gxsd-btn" onClick={this.refresh}>
							<Icon name="refresh" size={13} />
							{t("act.refresh")}
						</button>
					</header>

					<div className="gxsd-body">
						{/* key=view remounts on switch: a fresh (cached) fetch, clean state */}
						<Active
							key={active.key}
							t={t}
							refreshNonce={this.state.refreshNonce}
							__testData={active.key === viewKey ? this.props.__testData : undefined}
						/>
					</div>
				</div>
			</div>
		);
	}
}
