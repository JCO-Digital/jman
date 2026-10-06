import { ref } from "vue";
import { defineStore } from "pinia";
import { useAuthStore } from "./auth";
import { useDataStore } from "./data";
import type {
	ManagedServer,
	ManagedServerPayload,
	ManagedSite,
	ManagedSitePayload,
	SpinupWPSyncResult,
	TestConnectionResult,
} from "../types";
import { BASE_URL, handleErrorResponse, apiFetch } from "../utils/api";

/** Provider whose sites/servers are owned by the SpinupWP sync and read-only. */
export const READ_ONLY_PROVIDER = "spinupwp";

export const useManagedSitesStore = defineStore("managedSites", () => {
	const authStore = useAuthStore();
	const dataStore = useDataStore();

	const sites = ref<ManagedSite[]>([]);
	const servers = ref<ManagedServer[]>([]);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	// ---------------------------------------------------------------------------
	// Fetch sites and servers
	// ---------------------------------------------------------------------------

	async function fetchAll() {
		if (!authStore.isAuthenticated) return;
		isLoading.value = true;
		error.value = null;
		try {
			const [sitesRes, serversRes] = await Promise.all([
				apiFetch(`${BASE_URL}/sites?format=managed`),
				apiFetch(`${BASE_URL}/servers?format=managed`),
			]);
			if (sitesRes.status === 401 || serversRes.status === 401) {
				authStore.logout();
				return;
			}
			if (!sitesRes.ok) await handleErrorResponse(sitesRes);
			if (!serversRes.ok) await handleErrorResponse(serversRes);
			sites.value = await sitesRes.json();
			servers.value = await serversRes.json();
		} catch (e: any) {
			error.value = e.message || "Failed to load sites and servers";
			console.error("Error fetching managed sites:", e);
		} finally {
			isLoading.value = false;
		}
	}

	// Re-fetch this store and the global data store so the main Sites view
	// and its session cache pick up the change.
	async function refreshAfterMutation() {
		await Promise.all([fetchAll(), dataStore.refreshData()]);
	}

	async function request<T>(
		path: string,
		method: string,
		payload?: unknown,
	): Promise<T> {
		const headers: Record<string, string> = {};
		if (payload !== undefined) headers["Content-Type"] = "application/json";
		const res = await apiFetch(`${BASE_URL}${path}`, {
			method,
			headers,
			body: payload !== undefined ? JSON.stringify(payload) : undefined,
		});
		if (!res.ok) {
			await handleErrorResponse(res);
		}
		return res.json();
	}

	// ---------------------------------------------------------------------------
	// Site actions
	// ---------------------------------------------------------------------------

	async function createSite(
		payload: ManagedSitePayload,
	): Promise<ManagedSite> {
		const created = await request<ManagedSite>("/sites", "POST", payload);
		await refreshAfterMutation();
		return created;
	}

	async function updateSite(
		id: string,
		payload: ManagedSitePayload,
	): Promise<ManagedSite> {
		const updated = await request<ManagedSite>(
			`/sites/${id}`,
			"PUT",
			payload,
		);
		await refreshAfterMutation();
		return updated;
	}

	async function deleteSite(id: string): Promise<void> {
		await request(`/sites/${id}`, "DELETE");
		await refreshAfterMutation();
	}

	function testConnection(id: string): Promise<TestConnectionResult> {
		return request<TestConnectionResult>(
			`/sites/${id}/test-connection`,
			"POST",
		);
	}

	// ---------------------------------------------------------------------------
	// Server actions
	// ---------------------------------------------------------------------------

	async function createServer(
		payload: ManagedServerPayload,
	): Promise<ManagedServer> {
		const created = await request<ManagedServer>(
			"/servers",
			"POST",
			payload,
		);
		await fetchAll();
		return created;
	}

	async function deleteServer(id: string): Promise<void> {
		await request(`/servers/${id}`, "DELETE");
		await fetchAll();
	}

	async function syncSpinupWP(): Promise<SpinupWPSyncResult> {
		const result = await request<SpinupWPSyncResult>(
			"/providers/spinupwp/sync",
			"POST",
		);
		await refreshAfterMutation();
		return result;
	}

	function clearCache() {
		sites.value = [];
		servers.value = [];
		error.value = null;
	}

	return {
		// State
		sites,
		servers,
		isLoading,
		error,
		// Actions
		fetchAll,
		createSite,
		updateSite,
		deleteSite,
		testConnection,
		createServer,
		deleteServer,
		syncSpinupWP,
		clearCache,
	};
});
