/**
 * Formats a byte count into a human-readable string (e.g. "1.5 GB").
 */
export function formatBytes(bytes: number, decimals = 1): string {
	if (bytes === 0) return "0 B";
	const k = 1024;
	const dm = decimals < 0 ? 0 : decimals;
	const sizes = ["B", "KB", "MB", "GB", "TB"];
	const i = Math.floor(Math.log(bytes) / Math.log(k));
	return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + " " + sizes[i];
}

/**
 * Removes HTML tags, decodes HTML entities, and trims surrounding whitespace.
 * Mirrors utils.CleanHTML on the Go side, which normalizes the same external
 * feed text (WordPress.org, vulnerability databases) for CLI and Slack output.
 */
export function cleanHtml(text: string | null | undefined): string {
	if (!text) return "";
	// A detached textarea parses entities without interpreting markup, so the
	// tags are stripped first and nothing in the string can execute.
	const el = document.createElement("textarea");
	el.innerHTML = text.replace(/<[^>]*>/g, "");
	return el.value.replace(/[-–—]+/g, "-").trim();
}

// The wpvulnerability.net feed reports CVSS severity as a single-letter code,
// matching the letter codes it uses for the other vector components.
const CVSS_SEVERITY_LABELS: Record<string, string> = {
	n: "None",
	l: "Low",
	m: "Medium",
	h: "High",
	c: "Critical",
};

/**
 * Expands a CVSS severity code into a readable label. Values that are already
 * spelled out are returned unchanged, so feeds using either form both work.
 */
export function cvssSeverityLabel(severity: string | null | undefined): string {
	if (!severity) return "";
	const label = CVSS_SEVERITY_LABELS[severity.trim().toLowerCase()];
	return label ?? severity;
}
