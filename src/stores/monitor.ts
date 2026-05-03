import { ref, computed } from "vue";
import { defineStore } from "pinia";
import { useAuthStore } from "./auth";
import type { MonitorHistory, MonitorStatus, IgnoredSite } from "../types";

const BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api";

export const useMonitorStore = defineStore("monitor", () => {
	const authStore = useAuthStore();

	const history = ref<MonitorHistory[]>([]);
	const currentStatus = ref<Record<string, MonitorStatus>>({});
	const ignoredDomains = ref<IgnoredSite[]>([]);
	const isLoadingHistory = ref(false);
	const historyFetched = ref(false);
	const isLoadingIgnored = ref(false);

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
			const res = await fetch(`${BASE_URL}/monitor/history?hours=${hours}`, {
				headers: authStore.authHeader,
			});
			const data = await res.json();
			console.log("Monitor history:", data);
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
			const data = await res.json();
			console.log(`Monitor status ${domain ? `for ${domain}` : "all"}:`, data);
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
	 * GET /api/monitor/ignored
	 * Returns a list of currently ignored sites.
	 */
	async function fetchIgnored() {
		isLoadingIgnored.value = true;
		try {
			const res = await fetch(`${BASE_URL}/monitor/ignored`, {
				headers: authStore.authHeader,
			});
			const data = await res.json();
			console.log("Ignored sites:", data);
			ignoredDomains.value = data;
			return data;
		} catch (error) {
			console.error("Failed to fetch ignored sites:", error);
			throw error;
		} finally {
			isLoadingIgnored.value = false;
		}
	}

	/**
	 * POST /api/monitor/ignored
	 * Adds a site to the ignore list.
	 * Body: {"domain": "example.com", "reason": "Maintenance"}
	 */
	async function addIgnored(domain: string, reason: string) {
		try {
			const res = await fetch(`${BASE_URL}/monitor/ignored`, {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					...authStore.authHeader,
				},
				body: JSON.stringify({ domain, reason }),
			});

			if (!res.ok) {
				throw new Error("Failed to add site to ignore list");
			}

			const data = await res.json();
			console.log("Added to ignore list:", data);
			await fetchIgnored();
			return data;
		} catch (error) {
			console.error(`Failed to ignore site ${domain}:`, error);
			throw error;
		}
	}

	/**
	 * DELETE /api/monitor/ignored/{domain}
	 * Removes a site from the ignore list.
	 */
	async function removeIgnored(domain: string) {
		try {
			const res = await fetch(`${BASE_URL}/monitor/ignored/${domain}`, {
				method: "DELETE",
				headers: authStore.authHeader,
			});

			if (!res.ok) {
				throw new Error("Failed to remove site from ignore list");
			}

			// Check if response is empty or JSON
			const contentType = res.headers.get("content-type");
			let data = null;
			if (contentType && contentType.includes("application/json")) {
				data = await res.json();
			} else {
				data = await res.text();
			}

			console.log(`Removed ${domain} from ignore list:`, data);
			await fetchIgnored();
			return data;
		} catch (error) {
			console.error(`Failed to remove site ${domain} from ignore list:`, error);
			throw error;
		}
	}

	return {
		history,
		historyFetched,
		historyByDomain,
		currentStatus,
		ignoredDomains,
		isLoadingHistory,
		isLoadingIgnored,
		fetchHistory,
		ensureHistory,
		fetchStatus,
		fetchIgnored,
		addIgnored,
		removeIgnored,
	};
});
