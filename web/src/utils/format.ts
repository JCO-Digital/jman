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

/** Human-readable name for a site/server provider ("spinupwp" → "SpinupWP"). */
export function providerLabel(provider: string | null | undefined): string {
	switch (provider) {
		case "spinupwp":
			return "SpinupWP";
		case "wpengine":
			return "WP Engine";
		case "manual":
			return "Manual";
		case null:
		case undefined:
		case "":
			return "—";
		default:
			return provider;
	}
}

/**
 * Formats a duration in milliseconds compactly, e.g. "45s", "3m 05s",
 * "1h 02m".
 */
export function formatElapsed(ms: number): string {
	const total = Math.max(0, Math.floor(ms / 1000));
	const h = Math.floor(total / 3600);
	const m = Math.floor((total % 3600) / 60);
	const s = total % 60;
	const pad = (n: number) => String(n).padStart(2, "0");
	if (h > 0) return `${h}h ${pad(m)}m`;
	if (m > 0) return `${m}m ${pad(s)}s`;
	return `${s}s`;
}

/** Formats a past timestamp relative to now, e.g. "just now", "5 min ago". */
export function formatRelativeTime(timestamp: number, now: number): string {
	const sec = Math.floor((now - timestamp) / 1000);
	if (sec < 45) return "just now";
	const min = Math.round(sec / 60);
	if (min < 60) return `${min} min ago`;
	const hours = Math.round(min / 60);
	if (hours < 24) return `${hours} h ago`;
	return new Date(timestamp).toLocaleDateString();
}

/**
 * The version a cached plugin can be updated to, or "" if none. WP-CLI
 * reports no update as an empty update version; "none" is treated the same.
 */
export function availablePluginUpdate(plugin: {
	update?: string;
	version?: string;
}): string {
	const u = plugin.update ?? "";
	return u && u !== "none" && u !== plugin.version ? u : "";
}
