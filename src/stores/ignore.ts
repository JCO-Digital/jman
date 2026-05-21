import { ref, computed } from "vue";
import { defineStore } from "pinia";
import type {
	IgnoreEntry,
	CreateIgnorePayload,
	UpdateIgnorePayload,
} from "../types";
import { useAuthStore } from "./auth";

const BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api";

export const useIgnoreStore = defineStore("ignore", () => {
	const authStore = useAuthStore();
	const ignoreEntries = ref<IgnoreEntry[]>([]);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	const ignoreLookups = computed(() => {
		const monitor = {
			sites: new Set<string>(),
			servers: new Map<string, Set<number>>(),
		};

		for (const entry of ignoreEntries.value) {
			if (entry.use_for_monitor) {
				if (entry.type === "site") monitor.sites.add(entry.target);
				else if (entry.type === "server") {
					monitor.servers.set(
						entry.target,
						new Set(entry.negated_site_ids || []),
					);
				}
			}
		}

		return { monitor };
	});

	async function fetchIgnoreEntries(type?: string) {
		isLoading.value = true;
		error.value = null;
		try {
			const url = new URL(`${BASE_URL}/ignore`, window.location.origin);
			if (type) url.searchParams.append("type", type);

			const res = await fetch(url.toString(), {
				headers: authStore.authHeader,
			});

			if (!res.ok) {
				if (res.status === 401) {
					authStore.logout();
					return;
				}
				throw new Error("Failed to fetch ignore entries");
			}

			const data = await res.json();
			ignoreEntries.value = data || [];
			return ignoreEntries.value;
		} catch (e: any) {
			error.value = e.message;
			console.error("fetchIgnoreEntries error:", e);
			throw e;
		} finally {
			isLoading.value = false;
		}
	}

	async function addIgnoreEntry(payload: CreateIgnorePayload) {
		isLoading.value = true;
		error.value = null;
		try {
			const res = await fetch(`${BASE_URL}/ignore`, {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					...authStore.authHeader,
				},
				body: JSON.stringify(payload),
			});

			if (!res.ok) {
				const data = await res.json().catch(() => ({}));
				throw new Error(data.error || "Failed to add ignore entry");
			}

			const data = await res.json();
			await fetchIgnoreEntries();
			return data;
		} catch (e: any) {
			error.value = e.message;
			throw e;
		} finally {
			isLoading.value = false;
		}
	}

	async function updateIgnoreEntry(id: number, payload: UpdateIgnorePayload) {
		isLoading.value = true;
		error.value = null;
		try {
			const res = await fetch(`${BASE_URL}/ignore/${id}`, {
				method: "PATCH",
				headers: {
					"Content-Type": "application/json",
					...authStore.authHeader,
				},
				body: JSON.stringify(payload),
			});

			if (!res.ok) {
				const data = await res.json().catch(() => ({}));
				throw new Error(data.error || "Failed to update ignore entry");
			}

			const data = await res.json();
			await fetchIgnoreEntries();
			return data;
		} catch (e: any) {
			error.value = e.message;
			throw e;
		} finally {
			isLoading.value = false;
		}
	}

	async function deleteIgnoreEntry(id: number) {
		isLoading.value = true;
		error.value = null;
		try {
			const res = await fetch(`${BASE_URL}/ignore/${id}`, {
				method: "DELETE",
				headers: authStore.authHeader,
			});

			if (!res.ok) {
				const data = await res.json().catch(() => ({}));
				throw new Error(data.error || "Failed to delete ignore entry");
			}

			await fetchIgnoreEntries();
		} catch (e: any) {
			error.value = e.message;
			throw e;
		} finally {
			isLoading.value = false;
		}
	}

	/**
	 * Returns true if monitoring is ignored for a site.
	 */
	function isMonitoringIgnored(siteId?: number, serverId?: number): boolean {
		const lookups = ignoreLookups.value.monitor;

		if (siteId !== undefined && lookups.sites.has(siteId.toString())) {
			return true;
		}

		if (serverId !== undefined) {
			const serverIdStr = serverId.toString();
			const negatedSites = lookups.servers.get(serverIdStr);
			if (negatedSites) {
				if (siteId !== undefined && negatedSites.has(siteId)) {
					return false;
				}
				return true;
			}
		}

		return false;
	}

	return {
		ignoreEntries,
		isLoading,
		error,
		fetchIgnoreEntries,
		addIgnoreEntry,
		updateIgnoreEntry,
		deleteIgnoreEntry,
		isMonitoringIgnored,
	};
});
