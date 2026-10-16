import { ref, computed } from "vue";
import { defineStore } from "pinia";
import { useAuthStore } from "./auth";
import type { MonitorCheck, MonitorHistory, MonitorStatus } from "../types";
import { BASE_URL, apiFetch } from "../utils/api";

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

	return {
		checks,
		defaultCheckBySite,
		fetchChecks,
		ensureChecks,
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
