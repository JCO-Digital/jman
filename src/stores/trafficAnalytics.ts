import { ref } from "vue";
import { defineStore } from "pinia";
import { useAuthStore } from "./auth";
import type { SiteTrafficPeriod } from "../types";
import { BASE_URL } from "../utils/api";

export type TrafficPeriod = "hourly" | "daily";

function cacheKey(siteId: number, period: TrafficPeriod, days: number) {
	return `${siteId}:${period}:${days}`;
}

export const useTrafficAnalyticsStore = defineStore("trafficAnalytics", () => {
	const authStore = useAuthStore();

	// Cache keyed by `${siteId}:${period}:${days}` so toggling back and forth
	// doesn't refetch already-loaded data.
	const cache = ref<Record<string, SiteTrafficPeriod[]>>({});
	const isLoading = ref<Record<string, boolean>>({});
	const error = ref<Record<string, string | null>>({});

	/**
	 * GET /api/sites/{id}/traffic?period=hourly|daily&days=N
	 * Fetches hourly or daily visitor traffic for a site, oldest first.
	 * Results are cached per (siteId, period, days) combination.
	 */
	async function fetchTraffic(
		siteId: number,
		period: TrafficPeriod = "hourly",
		days: number = 7,
	): Promise<SiteTrafficPeriod[] | undefined> {
		const key = cacheKey(siteId, period, days);

		if (cache.value[key]) {
			return cache.value[key];
		}

		if (!authStore.isAuthenticated) return;

		isLoading.value[key] = true;
		error.value[key] = null;
		try {
			const res = await fetch(
				`${BASE_URL}/sites/${siteId}/traffic?period=${period}&days=${days}`,
				{
					headers: authStore.authHeader,
				},
			);
			if (res.status === 401) {
				authStore.logout();
				return;
			}
			if (!res.ok) throw new Error("Failed to fetch site traffic");
			const data: SiteTrafficPeriod[] = await res.json();
			cache.value[key] = data || [];
			return cache.value[key];
		} catch (e: any) {
			error.value[key] = e.message || "Failed to load site traffic";
			console.error("Error fetching site traffic:", e);
			throw e;
		} finally {
			isLoading.value[key] = false;
		}
	}

	function getTraffic(
		siteId: number,
		period: TrafficPeriod,
		days: number,
	): SiteTrafficPeriod[] | undefined {
		return cache.value[cacheKey(siteId, period, days)];
	}

	function isLoadingTraffic(
		siteId: number,
		period: TrafficPeriod,
		days: number,
	): boolean {
		return !!isLoading.value[cacheKey(siteId, period, days)];
	}

	function getError(
		siteId: number,
		period: TrafficPeriod,
		days: number,
	): string | null {
		return error.value[cacheKey(siteId, period, days)] ?? null;
	}

	function clearCache() {
		cache.value = {};
		isLoading.value = {};
		error.value = {};
	}

	return {
		// Actions
		fetchTraffic,
		// Reactive readers
		getTraffic,
		isLoadingTraffic,
		getError,
		clearCache,
	};
});
