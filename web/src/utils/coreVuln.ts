/**
 * Display helpers for WordPress core vulnerabilities. The feed names them
 * like "WordPress Core 6.6.0 - 7.1.3 - Authenticated (Contributor+) Stored
 * Cross-Site Scripting via …": the affected range, who can exploit it, and
 * what it is. Core entries have no descriptions, so the name is all there is.
 */

/** One core vulnerability, from either the vulnerability or exposure APIs. */
export interface CoreVulnItem {
	uuid: string;
	name: string;
	link?: string;
	cvss: number | null;
	suppressed?: boolean;
}

export interface CoreVulnTitle {
	/** What it is, without the range and access prefix. */
	title: string;
	/** "Unauthenticated", a role like "Contributor+", or "" if not stated. */
	access: string;
	/** The affected range, like "6.6.0 – 7.1.3", or "". */
	range: string;
}

const PREFIX_RE = /^WordPress Core\s+([\d.]+)\s*-\s*([\d.]+)\s*-\s*/i;
const ACCESS_RE = /^(Unauthenticated|Authenticated(?:\s*\(([^)]+)\))?)\s+/i;

export function parseCoreVulnName(name: string): CoreVulnTitle {
	let rest = name.trim();
	let range = "";
	const prefix = PREFIX_RE.exec(rest);
	if (prefix) {
		range = `${prefix[1]} – ${prefix[2]}`;
		rest = rest.slice(prefix[0].length);
	}
	let access = "";
	const acc = ACCESS_RE.exec(rest);
	if (acc) {
		access = acc[2]
			? acc[2]
			: /^unauth/i.test(acc[1] ?? "")
				? "Unauthenticated"
				: "Authenticated";
		rest = rest.slice(acc[0].length);
	}
	const title = rest.replace(/Cross-Site Scripting/g, "XSS") || name;
	return { title, access, range };
}

// Lower means easier to exploit.
const ACCESS_RANK: Record<string, number> = {
	unauthenticated: 0,
	"subscriber+": 1,
	"customer+": 1,
	"contributor+": 2,
	"author+": 3,
	"shop manager+": 4,
	"editor+": 4,
	"custom+": 5,
	authenticated: 5,
	"administrator+": 6,
	"admin+": 6,
};

function accessRank(access: string): number {
	return ACCESS_RANK[access.toLowerCase()] ?? 7;
}

/**
 * Most serious first: scored ones by CVSS, then the easiest to exploit
 * (unauthenticated, then the lowest role), then by title.
 */
export function sortCoreVulns<T extends CoreVulnItem>(items: T[]): T[] {
	return items
		.map((item) => ({ item, parsed: parseCoreVulnName(item.name) }))
		.sort(
			(a, b) =>
				(b.item.cvss ?? -1) - (a.item.cvss ?? -1) ||
				accessRank(a.parsed.access) - accessRank(b.parsed.access) ||
				a.parsed.title.localeCompare(b.parsed.title),
		)
		.map(({ item }) => item);
}

/** The highest CVSS score among the items, or null if none is scored. */
export function maxCoreVulnCvss(items: CoreVulnItem[]): number | null {
	let max: number | null = null;
	for (const item of items) {
		if (item.cvss != null && (max == null || item.cvss > max)) {
			max = item.cvss;
		}
	}
	return max;
}

/** A feed vulnerability's CVSS score as a number, or null. */
export function cvssOf(v: { impact?: { cvss?: { score: string } } }) {
	const score = parseFloat(v.impact?.cvss?.score ?? "");
	return Number.isNaN(score) ? null : score;
}
