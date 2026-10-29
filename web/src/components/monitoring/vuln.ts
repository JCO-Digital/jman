import type {
	TaskPriority,
	VulnExposureGroup,
	VulnExposureKind,
	VulnExposureSite,
	VulnFixState,
	VulnMute,
} from "../../types";

/** The scan severity matching a CVSS score, for severityClass. */
export function cvssSeverity(cvss: number | null | undefined): string {
	if (cvss == null) return "";
	if (cvss >= 9) return "critical";
	if (cvss >= 7) return "high";
	if (cvss >= 4) return "medium";
	return "low";
}

export function formatCvss(cvss: number | null | undefined): string {
	return cvss == null ? "no score" : `CVSS ${cvss.toFixed(1)}`;
}

export function formatDate(iso?: string | null): string {
	if (!iso) return "-";
	return new Date(iso).toLocaleDateString(undefined, {
		day: "numeric",
		month: "short",
		year: "numeric",
	});
}

/** Human-readable reason a plugin was closed on wordpress.org. */
export function closedLabel(g: VulnExposureGroup): string {
	const reason = g.closed_reason?.replace(/-/g, " ");
	return reason ? `Closed: ${reason}` : "Closed";
}

/** Defaults for a task to update (or remove) a vulnerable plugin on a site. */
export function exposureTaskDefaults(
	g: VulnExposureGroup,
	s: VulnExposureSite,
) {
	const what = g.kind === "core" ? "WordPress core" : g.name;
	const action =
		s.fix_state === "fix_available"
			? `Update ${what} to ${s.fixed_in}`
			: `Decide on ${what} (no fix)`;
	const vulns = g.vulnerabilities
		.filter((v) => s.vuln_uuids.includes(v.uuid))
		.map(
			(v) =>
				`- ${v.name} (${formatCvss(v.cvss)})${v.link ? ` ${v.link}` : ""}`,
		);
	const severity = cvssSeverity(s.max_cvss);
	const priority: TaskPriority =
		severity === "critical" || severity === "high"
			? "high"
			: severity === "medium" || !severity
				? "medium"
				: "low";
	return {
		title: `${action} on ${s.site_name}`,
		description: [
			`${what} ${s.version} on ${s.site_name} has known vulnerabilities:`,
			...vulns,
		].join("\n"),
		priority,
		site_id: s.site_id,
		plugin_slug: g.kind === "plugin" ? g.slug : undefined,
	};
}

/** What is muted: one plugin (or core) on one site. */
export interface MuteTarget {
	kind: VulnExposureKind;
	slug: string;
	siteId: string;
	name: string;
	siteName: string;
	fixState: VulnFixState;
	fixedIn?: string;
	/** Set to confirm (renew) this mute instead of making a new one. */
	mute?: VulnMute;
}
