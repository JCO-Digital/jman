import type { ScanSeverity } from "../../types";

/** The rules a path acknowledgement can cover. */
const stateRules = new Set([
	"stray-file",
	"foreign-owner",
	"unreadable",
	"new-admin",
	"new-app-password",
]);

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
		default:
			return "info";
	}
}
