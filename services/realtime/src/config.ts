// Configuration loaded from environment variables.
// All required variables throw at startup if absent so misconfiguration is
// caught immediately rather than at first use.

export interface TenantValkey {
	tenant: string;
	url: string;
}

export interface Config {
	port: number;
	apiUrl: string;
	valkey: {
		// url is the PRIMARY tenant's Valkey — also where socket sessions live,
		// which is process-wide state and deliberately not per tenant.
		url: string;
		// tenants lists one Valkey connection per tenant, derived exactly the
		// way services/api derives them: the primary keeps REDIS_URL verbatim
		// and each further tenant gets the next logical database index. Both
		// sides reading PACA_TENANTS from the same environment is what keeps
		// them pointing at the same place; if they ever disagree, the API
		// publishes into a database nobody is listening to and events simply
		// stop — loudly enough to notice, unlike delivering to the wrong one.
		tenants: TenantValkey[];
	};
	cors: {
		origins: string[];
	};
	logLevel: string;
}

function requireEnv(name: string): string {
	const value = process.env[name];
	if (!value) throw new Error(`Missing required environment variable: ${name}`);
	return value;
}

export function loadConfig(): Config {
	return {
		port: parseInt(process.env.PORT ?? "3001", 10),
		// Internal API base URL (service-to-service, not via the public gateway).
		apiUrl: requireEnv("API_URL"),
		valkey: {
			url: requireEnv("REDIS_URL"),
			tenants: tenantValkeys(requireEnv("REDIS_URL")),
		},
		cors: {
			// Comma-separated list of allowed origins, e.g. "http://localhost:3000,https://app.example.com"
			origins: (process.env.CORS_ORIGINS ?? "http://localhost:3000")
				.split(",")
				.map((s) => s.trim())
				.filter(Boolean),
		},
		logLevel: process.env.LOG_LEVEL ?? "info",
	};
}

// tenantValkeys mirrors buildTenants in services/api/internal/config: the
// first tenant keeps the base URL untouched, and every further one moves to
// the next logical database index.
export function tenantValkeys(baseUrl: string): TenantValkey[] {
	const codes = (process.env.PACA_TENANTS ?? "")
		.split(",")
		.map((s) => s.trim().toLowerCase())
		.filter(Boolean);
	const primary = (process.env.OIDC_TENANT ?? "").trim().toLowerCase();
	if (primary && !codes.includes(primary)) codes.unshift(primary);
	if (primary) {
		// The primary leads the list whatever order it was written in — it is
		// the one holding the data that already exists.
		const rest = codes.filter((c) => c !== primary);
		codes.length = 0;
		codes.push(primary, ...rest);
	}
	if (codes.length === 0) return [{ tenant: "", url: baseUrl }];

	return codes.map((tenant, i) => {
		if (i === 0) return { tenant, url: baseUrl };
		const explicit =
			process.env[`PACA_TENANT_REDIS_${tenant.replace(/-/g, "_").toUpperCase()}`];
		if (explicit) return { tenant, url: explicit };
		const u = new URL(baseUrl);
		u.pathname = `/${i}`;
		return { tenant, url: u.toString() };
	});
}
