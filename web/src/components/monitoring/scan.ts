import type { ScanFinding, ScanSeverity } from "../../types";

/**
 * The rules a path acknowledgement can cover, per site. unknown-plugin
 * isn't one: a plugin is trusted on every site by allowlisting its slug.
 */
const stateRules = new Set([
	"stray-file",
	"foreign-owner",
	"unreadable",
	"new-admin",
	"new-app-password",
]);

/** A plugin that isn't on wordpress.org or from J&Co Digital. */
export const unknownPluginRule = "unknown-plugin";

export function isStateRule(rule: string): boolean {
	return stateRules.has(rule);
}

/** The status-badge class for a finding or incident severity. */
export function severityClass(severity?: ScanSeverity | string): string {
	switch (severity) {
		case "critical":
		case "high":
			return "error";
		case "medium":
			return "warning";
		case "info":
			return "";
		default:
			return "info";
	}
}

const severityRanks: Record<string, number> = {
	info: 1,
	low: 2,
	medium: 3,
	high: 4,
	critical: 5,
};

/** Orders severities like jman-api: 1 for info to 5 for critical. */
export function severityRank(severity?: string): number {
	return severityRanks[severity ?? ""] ?? 0;
}

/** The worst severity of findings, or "" if there are none. */
export function worstSeverity(findings: { severity: string }[]): string {
	let worst = "";
	for (const f of findings) {
		if (severityRank(f.severity) > severityRank(worst)) worst = f.severity;
	}
	return worst;
}

/**
 * Whether a finding's hash can be allowlisted: a file's content, or an
 * unknown plugin's slug, unless the plugin poses as WordPress (high).
 */
export function canAllowlist(f: ScanFinding): boolean {
	if (!f.sha256) return false;
	if (f.rule === unknownPluginRule) return f.severity !== "high";
	return f.engine !== "api";
}

/** The plugin slug of an unknown-plugin finding's or entry's path. */
export function pluginSlug(path: string): string {
	return path.replace(/^wp-content\/plugins\//, "");
}

/** The plugin name from an unknown-plugin snippet ("name: …" line). */
export function pluginName(snippet?: string): string {
	const m = snippet?.match(/^name: (.*)$/m);
	return m?.[1]?.trim() ?? "";
}
