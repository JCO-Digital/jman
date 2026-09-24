import { ref } from "vue";
import { defineStore } from "pinia";
import { useAuthStore } from "./auth";
import type { Incident, IncidentsResponse } from "../types";
import { BASE_URL } from "../utils/api";

export const useIncidentStore = defineStore("incidents", () => {
	const authStore = useAuthStore();

	const activeIncidents = ref<Incident[]>([]);
	const historyIncidents = ref<Incident[]>([]);
	const activeCount = ref(0);
	const historyTotal = ref(0);
	const isLoading = ref(false);

	async function fetchActiveIncidents() {
		try {
			const res = await fetch(`${BASE_URL}/incidents?filter=active`, {
				headers: authStore.authHeader,
			});
			if (!res.ok) {
				if (res.status === 401) {
					authStore.logout();
					return;
				}
				throw new Error("Failed to fetch active incidents");
			}
			const data: IncidentsResponse = await res.json();
			activeIncidents.value = data.incidents || [];
			activeCount.value = data.active_count ?? activeIncidents.value.length;
			return activeIncidents.value;
		} catch (error) {
			console.error("Failed to fetch active incidents:", error);
			throw error;
		}
	}

	async function fetchHistoryIncidents(page: number = 1, limit: number = 20) {
		isLoading.value = true;
		try {
			const res = await fetch(
				`${BASE_URL}/incidents?filter=history&page=${page}&limit=${limit}`,
				{
					headers: authStore.authHeader,
				},
			);
			if (!res.ok) {
				if (res.status === 401) {
					authStore.logout();
					return;
				}
				throw new Error("Failed to fetch incident history");
			}
			const data: IncidentsResponse = await res.json();
			historyIncidents.value = data.incidents || [];
			historyTotal.value = data.total;
			activeCount.value = data.active_count;
			return historyIncidents.value;
		} catch (error) {
			console.error("Failed to fetch incident history:", error);
			throw error;
		} finally {
			isLoading.value = false;
		}
	}

	async function fetchActiveCount() {
		try {
			const res = await fetch(`${BASE_URL}/incidents?filter=active&limit=1`, {
				headers: authStore.authHeader,
			});
			if (!res.ok) return;
			const data: IncidentsResponse = await res.json();
			activeCount.value = data.active_count;
		} catch (error) {
			console.error("Failed to fetch incident count:", error);
		}
	}

	async function acknowledgeIncident(id: number) {
		const res = await fetch(`${BASE_URL}/incidents/${id}/acknowledge`, {
			method: "POST",
			headers: {
				...authStore.authHeader,
				"Content-Type": "application/json",
			},
		});
		if (!res.ok) {
			const err = await res.json().catch(() => ({ error: "Failed to acknowledge" }));
			throw new Error(err.error || "Failed to acknowledge incident");
		}
		const updated: Incident = await res.json();
		// Update in local state
		const idx = activeIncidents.value.findIndex((i) => i.id === id);
		if (idx !== -1) {
			activeIncidents.value[idx] = updated;
		}
		return updated;
	}

	async function closeIncident(id: number) {
		const res = await fetch(`${BASE_URL}/incidents/${id}/close`, {
			method: "POST",
			headers: {
				...authStore.authHeader,
				"Content-Type": "application/json",
			},
		});
		if (!res.ok) {
			const err = await res.json().catch(() => ({ error: "Failed to close incident" }));
			throw new Error(err.error || "Failed to close incident");
		}
		const updated: Incident = await res.json();
		// Remove from active list
		activeIncidents.value = activeIncidents.value.filter((i) => i.id !== id);
		activeCount.value = Math.max(0, activeCount.value - 1);
		return updated;
	}

	async function ignoreIncident(
		id: number,
		reason: string,
		useForMonitor: boolean = true,
		useForVuln: boolean = false,
	) {
		const res = await fetch(`${BASE_URL}/incidents/${id}/ignore`, {
			method: "POST",
			headers: {
				...authStore.authHeader,
				"Content-Type": "application/json",
			},
			body: JSON.stringify({
				reason,
				use_for_monitor: useForMonitor,
				use_for_vuln: useForVuln,
			}),
		});
		if (!res.ok) {
			const err = await res.json().catch(() => ({ error: "Failed to ignore site" }));
			throw new Error(err.error || "Failed to ignore site from incident");
		}
		const updated: Incident = await res.json();
		activeIncidents.value = activeIncidents.value.filter((i) => i.id !== id);
		activeCount.value = Math.max(0, activeCount.value - 1);
		return updated;
	}

	return {
		activeIncidents,
		historyIncidents,
		activeCount,
		historyTotal,
		isLoading,
		fetchActiveIncidents,
		fetchHistoryIncidents,
		fetchActiveCount,
		acknowledgeIncident,
		closeIncident,
		ignoreIncident,
	};
});
