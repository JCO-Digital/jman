import { ref, computed } from "vue";
import { defineStore } from "pinia";
import { useAuthStore } from "./auth";
import type { MonitorHistory, MonitorStatus } from "../types";

const BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api";

export const useMonitorStore = defineStore("monitor", () => {
	const authStore = useAuthStore();

	const history = ref<MonitorHistory[]>([]);
	const currentStatus = ref<Record<string, MonitorStatus>>({});
	const isLoadingHistory = ref(false);
	const historyFetched = ref(false);

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
			const res = await fetch(
				`${BASE_URL}/monitor/history?hours=${hours}`,
				{
					headers: authStore.authHeader,
				},
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
			const res = await fetch(url, {
				headers: authStore.authHeader,
			});
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

	return {
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
