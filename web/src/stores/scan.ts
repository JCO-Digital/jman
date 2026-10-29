import { ref } from "vue";
import { defineStore } from "pinia";
import type {
	Incident,
	IncidentsResponse,
	ScanAllowlistEntry,
	ScanFinding,
	ScanPathAck,
} from "../types";
import { BASE_URL, apiFetch, handleErrorResponse } from "../utils/api";

export interface ScanFindingQuery {
	site_id?: string;
	incident_id?: number;
	status?: string;
	sha256?: string;
	unassigned?: boolean;
	limit?: number;
}

/** File scan findings, security incidents and their triage. */
export const useScanStore = defineStore("scan", () => {
	const activeIncidents = ref<Incident[]>([]);
	const historyIncidents = ref<Incident[]>([]);
	const historyTotal = ref(0);
	const allowlist = ref<ScanAllowlistEntry[]>([]);
	const pathAcks = ref<ScanPathAck[]>([]);

	async function getJSON<T>(path: string): Promise<T> {
		const res = await apiFetch(`${BASE_URL}${path}`);
		if (!res.ok) await handleErrorResponse(res);
		return res.json();
	}

	async function send<T>(
		method: string,
		path: string,
		body?: unknown,
	): Promise<T | null> {
		const res = await apiFetch(`${BASE_URL}${path}`, {
			method,
			headers: { "Content-Type": "application/json" },
			body: body === undefined ? undefined : JSON.stringify(body),
		});
		if (!res.ok) await handleErrorResponse(res);
		return res.status === 204 ? null : res.json();
	}

	async function fetchActiveIncidents() {
		const data = await getJSON<IncidentsResponse>(
			"/incidents?type=security&filter=active&limit=200",
		);
		activeIncidents.value = data.incidents;
	}

	async function fetchHistoryIncidents(page = 1, limit = 20) {
		const data = await getJSON<IncidentsResponse>(
			`/incidents?type=security&filter=history&page=${page}&limit=${limit}`,
		);
		historyIncidents.value = data.incidents;
		historyTotal.value = data.total;
	}

	/** The number of sites with a file scan. */
	async function fetchScannedSiteCount(): Promise<number> {
		const data = await getJSON<{ scans: unknown[] | null }>("/scan/sites");
		return data.scans?.length ?? 0;
	}

	async function fetchFindings(q: ScanFindingQuery): Promise<ScanFinding[]> {
		const params = new URLSearchParams();
		for (const [k, v] of Object.entries(q)) {
			if (v === undefined || v === false) continue;
			params.set(k, v === true ? "1" : String(v));
		}
		const data = await getJSON<{ findings: ScanFinding[] }>(
			`/scan/findings?${params}`,
		);
		return data.findings;
	}

	/** Dismisses findings; one reported again opens again. */
	async function closeFindings(ids: number[]) {
		return send<{ closed: number }>("POST", "/scan/findings/close", {
			finding_ids: ids,
		});
	}

	/** Removes findings' snippets and details, which quote the files. */
	async function redactFindings(ids: number[]) {
		return send<{ redacted: number }>("POST", "/scan/findings/redact", {
			finding_ids: ids,
		});
	}

	/** Trusts a finding's file hash on every site. */
	async function addAllowlist(
		findingId: number,
		reason: string,
		closeMatching: boolean,
	) {
		return send<{ entry: ScanAllowlistEntry; allowlisted: number }>(
			"POST",
			"/scan/allowlist",
			{ finding_id: findingId, reason, close_matching: closeMatching },
		);
	}

	async function fetchAllowlist(includeRemoved = false) {
		const data = await getJSON<{ entries: ScanAllowlistEntry[] }>(
			`/scan/allowlist${includeRemoved ? "?include_removed=1" : ""}`,
		);
		allowlist.value = data.entries;
	}

	async function removeAllowlist(id: number) {
		await send("DELETE", `/scan/allowlist/${id}`);
	}

	/** Trusts the paths of stray, foreign-owned or unreadable files. */
	async function acknowledgePaths(ids: number[], reason: string) {
		return send<{ acks: ScanPathAck[]; acknowledged: number }>(
			"POST",
			"/scan/acks",
			{ finding_ids: ids, reason },
		);
	}

	async function fetchPathAcks(includeRemoved = false) {
		const data = await getJSON<{ acks: ScanPathAck[] }>(
			`/scan/acks${includeRemoved ? "?include_removed=1" : ""}`,
		);
		pathAcks.value = data.acks;
	}

	async function removePathAck(id: number) {
		await send("DELETE", `/scan/acks/${id}`);
	}

	return {
		activeIncidents,
		historyIncidents,
		historyTotal,
		allowlist,
		pathAcks,
		fetchActiveIncidents,
		fetchHistoryIncidents,
		fetchScannedSiteCount,
		fetchFindings,
		closeFindings,
		redactFindings,
		addAllowlist,
		fetchAllowlist,
		removeAllowlist,
		acknowledgePaths,
		fetchPathAcks,
		removePathAck,
	};
});
