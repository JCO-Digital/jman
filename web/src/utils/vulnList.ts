/**
 * Display helpers for compact vulnerability lists (VulnerabilityList.vue).
 *
 * Feed names repeat what the list already shows. Core ones read "WordPress
 * Core 6.6.0 - 7.1.3 - Authenticated (Contributor+) Stored Cross-Site
 * Scripting via …", plugin ones "Forminator <= 1.22.1 - Missing
 * Authorization on …" or "… < 6.2.5 is vulnerable to Cross Site Scripting
 * (XSS)": the product, the affected range, who can exploit it and what it
 * is. parseVulnName splits them so the list can show the last two.
 */
import type { Vulnerability } from "../types";

/** One vulnerability, from either the vulnerability or exposure APIs. */
export interface VulnListItem {
	uuid: string;
	name: string;
	link?: string;
	cvss: number | null;
	suppressed?: boolean;
	/** The first version without it, when the feed says. */
	fixedIn?: string;
	/** No fixed version exists. */
	unfixed?: boolean;
}

export interface VulnTitle {
	/** What it is, without the product, range and access prefix. */
	title: string;
	/** "Unauthenticated", a role like "Contributor+", or "" if not stated. */
	access: string;
	/** The affected range, like "6.6.0 – 7.1.3" or "<= 1.22.1", or "". */
	range: string;
}

const CORE_PREFIX_RE = /^WordPress Core\s+([\d.]+)\s*-\s*([\d.]+)\s*-\s*/i;
// "Plugin <= 1.2.3 - ", "Plugin < 1.2.3 - ", "Plugin >= 1.0 - < 1.2.3 - ",
// "Plugin 2.6-2.6.5 - "
const VERSION_PREFIX_RE =
	/^.*?\s((?:>=?\s*v?\d[\w.]*\s*-\s*)?<=?\s*v?\d[\w.]*|\d[\w.]*\s*-\s*\d[\w.]*)\s+-\s+/;
// "WordPress X Plugin < 6.2.5 is vulnerable to Cross Site Scripting (XSS)"
const VULNERABLE_TO_RE =
	/^.*?\s((?:<=?|>=?)\s*v?\d[\w.]*)?\s*is vulnerable to\s+/i;
const ACCESS_RE = /^(Unauthenticated|Authenticated(?:\s*\(([^)]+)\))?)\s+/i;

export function parseVulnName(name: string): VulnTitle {
	let rest = name.trim();
	let range = "";
	const core = CORE_PREFIX_RE.exec(rest);
	const version = core ? null : VERSION_PREFIX_RE.exec(rest);
	const vulnerableTo = core || version ? null : VULNERABLE_TO_RE.exec(rest);
	if (core) {
		range = `${core[1]} – ${core[2]}`;
		rest = rest.slice(core[0].length);
	} else if (version) {
		range = (version[1] ?? "").replace(/\s+/g, " ");
		rest = rest.slice(version[0].length);
	} else if (vulnerableTo) {
		range = (vulnerableTo[1] ?? "").replace(/\s+/g, " ");
		rest = rest.slice(vulnerableTo[0].length);
		rest = rest.charAt(0).toUpperCase() + rest.slice(1);
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
export function sortVulns<T extends { name: string; cvss: number | null }>(
	items: T[],
): T[] {
	return items
		.map((item) => ({ item, parsed: parseVulnName(item.name) }))
		.sort(
			(a, b) =>
				(b.item.cvss ?? -1) - (a.item.cvss ?? -1) ||
				accessRank(a.parsed.access) - accessRank(b.parsed.access) ||
				a.parsed.title.localeCompare(b.parsed.title),
		)
		.map(({ item }) => item);
}

/** The highest CVSS score among the items, or null if none is scored. */
export function maxVulnCvss(items: { cvss: number | null }[]): number | null {
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

/**
 * A feed vulnerability as a list item. Plugin names are "Plugin [slug] <
 * 1.2.3", so the first source name that isn't a bare CVE ID is used, as
 * the backend does for alerts.
 */
export function toVulnListItem(v: Vulnerability): VulnListItem {
	const sourceName =
		v.source.find((s) => s.name && !s.name.startsWith("CVE-"))?.name ??
		v.source.find((s) => s.name)?.name;
	const op = v.operator;
	return {
		uuid: v.uuid,
		name: sourceName || v.name,
		link: v.source.find((s) => s.link)?.link,
		cvss: cvssOf(v),
		suppressed: v.suppressed,
		fixedIn:
			op.max_operator === "lt" && op.max_version
				? op.max_version
				: undefined,
		unfixed: op.unfixed === "1",
	};
}

/**
 * Compares dotted version strings numerically ("1.10" > "1.9"); suffixes
 * like "-beta" compare as text. Negative, zero or positive like a sort
 * comparator.
 */
export function compareVersions(a: string, b: string): number {
	return a.localeCompare(b, undefined, { numeric: true });
}

/**
 * The version that fixes every item, or "" if some has no known fix (the
 * same rule as the backend's FixedIn).
 */
export function fixedInAll(items: VulnListItem[]): string {
	let fixed = "";
	for (const item of items) {
		if (!item.fixedIn || item.unfixed) return "";
		if (!fixed || compareVersions(item.fixedIn, fixed) > 0) {
			fixed = item.fixedIn;
		}
	}
	return fixed;
}
