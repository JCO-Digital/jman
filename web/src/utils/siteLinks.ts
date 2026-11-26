import type { Site } from "../types";

/** Domain labels that mark a non-production copy of a site. */
const ENV_LABELS = new Set([
	"staging",
	"stage",
	"stg",
	"dev",
	"develop",
	"development",
	"test",
	"testing",
	"qa",
	"uat",
	"preview",
	"old",
	"archive",
	"archived",
]);

const ENV_AFFIX = new RegExp(
	`^(?:(?:${[...ENV_LABELS].join("|")})-(.+)|(.+)-(?:${[...ENV_LABELS].join("|")}))$`,
);

function stripWww(domain: string): string {
	return domain.toLowerCase().replace(/^www\./, "");
}

/** Whether a site is a non-production environment that can be linked. */
export function isLinkableEnvironment(site: Site): boolean {
	return (
		site.environment === "staging" ||
		site.environment === "development" ||
		site.environment === "archived"
	);
}

/** A site name for matching: lowercase, without hyphens. */
function normalizeName(name: string): string {
	return name.toLowerCase().replace(/-/g, "");
}

/**
 * The production sites a staging, development or archived site is likely a
 * copy of, judging by its domain, best first; empty if none matches.
 *
 * - staging.example.com, www.dev.example.com → example.com
 * - staging-example.com, example-dev.com → example.com
 * - sitename.dev.bojaco.com, sitename.staging.bojaco.com → every
 *   production site named sitename (sitename.fi, www.sitename.com), with
 *   hyphens and case ignored (site-name.dev.bojaco.com → sitename.fi)
 */
export function suggestProductionSites(
	site: Site,
	productionSites: Site[],
): Site[] {
	const labels = stripWww(site.domain).split(".");
	const byDomain = new Map(
		productionSites.map((p) => [stripWww(p.domain), p]),
	);

	// Drop the environment labels, or the environment part of a label.
	const envAt = labels.findIndex((l) => ENV_LABELS.has(l));
	if (envAt >= 0) {
		const base = labels.filter((_, i) => i !== envAt).join(".");
		const match = byDomain.get(stripWww(base));
		if (match) return [match];
	}
	for (const [i, label] of labels.entries()) {
		const m = label.match(ENV_AFFIX);
		const stripped = m?.[1] ?? m?.[2];
		if (!stripped) continue;
		const base = [...labels];
		base[i] = stripped;
		const match = byDomain.get(base.join("."));
		if (match) return [match];
	}

	// A copy on the agency's domain (sitename.dev.bojaco.com): match the
	// name before the environment label to production sites' names.
	const name = labels[0];
	if (envAt === 1 && name) {
		const wanted = normalizeName(name);
		return productionSites
			.filter(
				(p) =>
					normalizeName(stripWww(p.domain).split(".")[0] ?? "") ===
					wanted,
			)
			.sort((a, b) => a.domain.localeCompare(b.domain));
	}
	return [];
}
