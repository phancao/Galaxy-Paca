import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
	dockPortalOrigin,
	portalOriginFrom,
	resetPortalOriginWarning,
	warnPortalOriginMissing,
} from "./portal-origin";

describe("portalOriginFrom", () => {
	it("returns the advertised origin without a trailing slash", () => {
		expect(portalOriginFrom({ portal_origin: "https://ai.spaxeai.com/" })).toBe(
			"https://ai.spaxeai.com",
		);
		expect(
			portalOriginFrom({ portal_origin: "http://portal.local:8080/some/path" }),
		).toBe("http://portal.local:8080");
	});

	it("never invents an estate: missing/empty/garbage → null, not a guess", () => {
		expect(portalOriginFrom(undefined)).toBeNull();
		expect(portalOriginFrom(null)).toBeNull();
		expect(portalOriginFrom({})).toBeNull();
		expect(portalOriginFrom({ portal_origin: "" })).toBeNull();
		expect(portalOriginFrom({ portal_origin: "   " })).toBeNull();
		expect(portalOriginFrom({ portal_origin: "not a url" })).toBeNull();
		expect(
			portalOriginFrom({ portal_origin: "javascript:alert(1)" }),
		).toBeNull();
	});
});

describe("dockPortalOrigin", () => {
	it("prefers the dock bundle's own origin when it is absolute", () => {
		expect(
			dockPortalOrigin("https://ai.spaxeai.com/dock.js", {
				portal_origin: "https://other.example",
			}),
		).toBe("https://ai.spaxeai.com");
	});

	it("uses the API's portal_origin for a relative dock src", () => {
		expect(
			dockPortalOrigin("/dock.js", { portal_origin: "https://ai.spaxeai.com" }),
		).toBe("https://ai.spaxeai.com");
	});

	it("relative dock src + no portal_origin → null (no relay to any estate)", () => {
		expect(dockPortalOrigin("/dock.js", {})).toBeNull();
		expect(dockPortalOrigin("/dock.js", undefined)).toBeNull();
	});
});

describe("no hardcoded estate anywhere in the portal-origin helpers", () => {
	const inputs: Parameters<typeof portalOriginFrom>[0][] = [
		undefined,
		null,
		{},
		{ portal_origin: "" },
	];
	for (const input of inputs) {
		it(`portalOriginFrom(${JSON.stringify(input) ?? "undefined"}) links to no other estate`, () => {
			const out = portalOriginFrom(input);
			expect(out).toBeNull();
			expect(String(out)).not.toContain("skyplatform");
		});
	}
	it("dockPortalOrigin with nothing to go on names no host at all", () => {
		expect(String(dockPortalOrigin("/dock.js", {}))).not.toMatch(/https?:/);
	});
});

describe("warnPortalOriginMissing", () => {
	beforeEach(() => resetPortalOriginWarning());
	afterEach(() => vi.restoreAllMocks());

	it("logs a clear error once per page, not on every render", () => {
		const err = vi.spyOn(console, "error").mockImplementation(() => {});
		warnPortalOriginMissing("UserMenu");
		warnPortalOriginMissing("LoginFormPanel");
		expect(err).toHaveBeenCalledTimes(1);
		expect(err.mock.calls[0]?.[0]).toContain("portal_origin");
		expect(err.mock.calls[0]?.[0]).toContain("UserMenu");
	});
});
