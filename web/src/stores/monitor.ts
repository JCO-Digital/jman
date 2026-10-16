import { ref, computed } from "vue";
import { defineStore } from "pinia";
import { useAuthStore } from "./auth";
import type {
	MonitorCheck,
	MonitorCheckPayload,
	MonitorCheckTestResult,
	MonitorHistory,
	MonitorStatus,
} from "../types";
import { BASE_URL, apiFetch, handleErrorResponse } from "../utils/api";

export const useMonitorStore = defineStore("monitor", () => {
	const authStore = useAuthStore();

	const history = ref<MonitorHistory[]>([]);
	const currentStatus = ref<Record<string, MonitorStatus>>({});
	const isLoadingHistory = ref(false);
	const historyFetched = ref(false);
	const checks = ref<MonitorCheck[]>([]);
	const checksFetched = ref(false);

	/** Default checks by site ID. */
	const defaultCheckBySite = computed(() => {
		const map = new Map<string, MonitorCheck>();
		for (const check of checks.value) {
			if (check.is_default && check.site_id)
				map.set(check.site_id, check);
		}
		return map;
	});

	const historyByDomain = computed(() => {
		const map = new Map<string, MonitorHistory[]>();
		for (const item of history.value) {
			if (!map.has(item.domain)) {
				map.set(item.domain, []);
			}
			map.get(item.domain)!.push(item);
		}
		return map;
	});

	/**
	 * GET /api/monitor/history?hours=48
	 * Returns aggregated status history for all sites.
	 */
	async function fetchHistory(hours: number = 48) {
		isLoadingHistory.value = true;
		try {
			const res = await apiFetch(
				`${BASE_URL}/monitor/history?hours=${hours}`,
			);
			if (!res.ok) {
				if (res.status === 401) {
					authStore.logout();
					return;
				}
				throw new Error("Failed to fetch monitor history");
			}
			const data = await res.json();
			history.value = data || [];
			historyFetched.value = true;
			return history.value;
		} catch (error) {
			console.error("Failed to fetch monitor history:", error);
			throw error;
		} finally {
			isLoadingHistory.value = false;
		}
	}

	async function ensureHistory() {
		if (!historyFetched.value && !isLoadingHistory.value) {
			await fetchHistory();
		}
	}

	/**
	 * GET /api/monitor/status?domain=...
	 * Returns current status for a specific site (or all sites if domain is omitted).
	 */
	async function fetchStatus(domain?: string) {
		try {
			const url = `${BASE_URL}/monitor/status${domain ? `?domain=${encodeURIComponent(domain)}` : ""}`;
			const res = await apiFetch(url);
			if (!res.ok) {
				if (res.status === 401) {
					authStore.logout();
					return;
				}
				throw new Error("Failed to fetch monitor status");
			}
			const data = await res.json();
			if (domain) {
				currentStatus.value[domain] = data;
			}
			return data;
		} catch (error) {
			console.error("Failed to fetch monitor status:", error);
			throw error;
		}
	}

	/**
	 * GET /api/monitor/checks
	 * Returns every monitor check.
	 */
	async function fetchChecks() {
		const res = await apiFetch(`${BASE_URL}/monitor/checks`);
		if (!res.ok) {
			if (res.status === 401) {
				authStore.logout();
				return;
			}
			throw new Error("Failed to fetch monitor checks");
		}
		checks.value = (await res.json()) || [];
		checksFetched.value = true;
		return checks.value;
	}

	async function ensureChecks() {
		if (!checksFetched.value) {
			await fetchChecks();
		}
	}

	/** POST /api/monitor/checks/{id}/pause or /resume */
	async function setCheckPaused(
		id: string,
		paused: boolean,
		reason?: string,
	) {
		const res = await apiFetch(
			`${BASE_URL}/monitor/checks/${encodeURIComponent(id)}/${paused ? "pause" : "resume"}`,
			{
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify(reason ? { reason } : {}),
			},
		);
		if (!res.ok) {
			const err = await res.json().catch(() => ({}));
			throw new Error(
				err.error || `Failed to ${paused ? "pause" : "resume"} check`,
			);
		}
		const updated: MonitorCheck = await res.json();
		const idx = checks.value.findIndex((c) => c.id === id);
		if (idx >= 0) checks.value[idx] = updated;
		return updated;
	}

	/** Current status of every check, by check ID. */
	const statusByCheck = ref<Record<string, MonitorStatus>>({});

	/** GET /api/monitor/status (all checks) */
	async function fetchCheckStatuses() {
		const res = await apiFetch(`${BASE_URL}/monitor/status`);
		if (!res.ok) await handleErrorResponse(res);
		const list: MonitorStatus[] = (await res.json()) || [];
		const map: Record<string, MonitorStatus> = {};
		for (const st of list) {
			if (st.check_id) map[st.check_id] = st;
		}
		statusByCheck.value = map;
	}

	function checkURL(id: string, suffix = "") {
		return `${BASE_URL}/monitor/checks/${encodeURIComponent(id)}${suffix}`;
	}

	/** Stores a check returned by the API in the list. */
	function upsertCheck(check: MonitorCheck) {
		const idx = checks.value.findIndex((c) => c.id === check.id);
		if (idx >= 0) checks.value[idx] = check;
		else checks.value.push(check);
		return check;
	}

	async function sendJSON(
		url: string,
		method: string,
		body?: unknown,
	): Promise<Response> {
		const res = await apiFetch(url, {
			method,
			headers: { "Content-Type": "application/json" },
			body: body === undefined ? undefined : JSON.stringify(body),
		});
		if (!res.ok) await handleErrorResponse(res);
		return res;
	}

	/** POST /api/monitor/checks */
	async function createCheck(payload: MonitorCheckPayload) {
		const res = await sendJSON(
			`${BASE_URL}/monitor/checks`,
			"POST",
			payload,
		);
		return upsertCheck(await res.json());
	}

	/** PATCH /api/monitor/checks/{id} */
	async function updateCheck(id: string, payload: MonitorCheckPayload) {
		const res = await sendJSON(checkURL(id), "PATCH", payload);
		return upsertCheck(await res.json());
	}

	/** POST /api/monitor/checks/{id}/reset */
	async function resetCheck(id: string) {
		const res = await sendJSON(checkURL(id, "/reset"), "POST");
		return upsertCheck(await res.json());
	}

	/** DELETE /api/monitor/checks/{id} */
	async function deleteCheck(id: string) {
		await sendJSON(checkURL(id), "DELETE");
		checks.value = checks.value.filter((c) => c.id !== id);
	}

	/** PUT /api/monitor/checks/{id}/secrets/{header} */
	async function setCheckSecret(id: string, header: string, value: string) {
		const res = await sendJSON(
			checkURL(id, `/secrets/${encodeURIComponent(header)}`),
			"PUT",
			{ value },
		);
		return upsertCheck(await res.json());
	}

	/** DELETE /api/monitor/checks/{id}/secrets/{header} */
	async function deleteCheckSecret(id: string, header: string) {
		const res = await sendJSON(
			checkURL(id, `/secrets/${encodeURIComponent(header)}`),
			"DELETE",
		);
		return upsertCheck(await res.json());
	}

	/** POST /api/monitor/checks/test: runs a check once, recording nothing. */
	async function testCheck(
		payload: MonitorCheckPayload & {
			id?: string;
			secret_headers?: Record<string, string>;
		},
	): Promise<MonitorCheckTestResult> {
		const res = await sendJSON(
			`${BASE_URL}/monitor/checks/test`,
			"POST",
			payload,
		);
		return res.json();
	}

	return {
		checks,
		defaultCheckBySite,
		statusByCheck,
		fetchChecks,
		fetchCheckStatuses,
		ensureChecks,
		createCheck,
		updateCheck,
		resetCheck,
		deleteCheck,
		setCheckSecret,
		deleteCheckSecret,
		testCheck,
		setCheckPaused,
		history,
		historyFetched,
		historyByDomain,
		currentStatus,
		isLoadingHistory,
		fetchHistory,
		ensureHistory,
		fetchStatus,
	};
});
