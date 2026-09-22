/**
 * Platform portal origin (scheme://host[:port], no path) — the ONE place the
 * SPA learns which Vortex estate it belongs to. The API advertises it on
 * /auth/config as `portal_origin` (derived from GALAXY_DOCK_SRC or an explicit
 * GALAXY_PORTAL_ORIGIN, T7).
 *
 * There is deliberately NO hardcoded default. Until 22/09/2026 three files
 * each carried `"https://ai.skyplatform.net"` as a fallback, so every estate
 * whose API did not yet advertise portal_origin silently sent its users'
 * logout / switch-workspace / dock-SSO links to Galaxy's OWN portal — a
 * fallback that "works" is worse than one that fails, because nothing ever
 * turns red. When the origin is unknown, callers must render NO link to any
 * other estate and say so on the console.
 */
export interface PortalOriginSource {
	portal_origin?: string | null;
}

/** Returns the portal origin without a trailing slash, or null when unknown. */
export function portalOriginFrom(
	config: PortalOriginSource | null | undefined,
): string | null {
	const raw = config?.portal_origin?.trim();
	if (!raw) {
		return null;
	}
	try {
		const url = new URL(raw);
		if (url.protocol !== "https:" && url.protocol !== "http:") {
			return null;
		}
		return url.origin;
	} catch {
		return null;
	}
}

/**
 * Portal origin for the Galaxy dock's /dock-sso relay: an absolute dock src
 * already names the platform; a relative one (same-origin gateway bridge)
 * must rely on what the API advertises. Null when neither says.
 */
export function dockPortalOrigin(
	dockSrc: string,
	config: PortalOriginSource | null | undefined,
): string | null {
	try {
		const url = new URL(dockSrc);
		if (url.protocol === "https:" || url.protocol === "http:") {
			return url.origin;
		}
	} catch {
		// relative dock src — fall through to the API's own answer
	}
	return portalOriginFrom(config);
}

let warned = false;
/** Log (once per page) that a portal link was withheld for want of an origin. */
export function warnPortalOriginMissing(where: string): void {
	if (warned) {
		return;
	}
	warned = true;
	console.error(
		`[portal-origin] ${where}: /auth/config carries no portal_origin — ` +
			"links to the platform portal are withheld rather than pointed at a " +
			"guessed estate. Set GALAXY_DOCK_SRC or GALAXY_PORTAL_ORIGIN on the API.",
	);
}

/** Test seam: forget that the warning was already emitted. */
export function resetPortalOriginWarning(): void {
	warned = false;
}
