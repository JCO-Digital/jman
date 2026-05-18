import { ref } from "vue";
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
	 * Returns true if the given target is ignored for the specified purpose.
	 */
	function isIgnored(params: {
		siteId?: number;
		serverId?: number;
		pluginSlug?: string;
		vulnUuid?: string;
		purpose: "monitor" | "vuln";
	}): boolean {
		const entries = ignoreEntries.value;

		return entries.some((entry) => {
			// Check purpose
			if (params.purpose === "monitor" && !entry.use_for_monitor)
				return false;
			if (params.purpose === "vuln" && !entry.use_for_vuln) return false;

			// Site match
			if (entry.type === "site" && params.siteId !== undefined) {
				if (entry.target === params.siteId.toString()) return true;
			}

			// Server match
			if (entry.type === "server" && params.serverId !== undefined) {
				if (entry.target === params.serverId.toString()) {
					// Check for negation
					if (params.siteId !== undefined && entry.negated_site_ids) {
						if (entry.negated_site_ids.includes(params.siteId)) {
							return false; // Negated, so NOT ignored
						}
					}
					return true;
				}
			}

			// Plugin match (vuln only)
			if (
				params.purpose === "vuln" &&
				entry.type === "plugin" &&
				params.pluginSlug !== undefined
			) {
				if (entry.target === params.pluginSlug) return true;
			}

			// Vulnerability match (vuln only)
			if (
				params.purpose === "vuln" &&
				entry.type === "vulnerability" &&
				params.vulnUuid !== undefined
			) {
				if (entry.target === params.vulnUuid) return true;
			}

			return false;
		});
	}

	return {
		ignoreEntries,
		isLoading,
		error,
		fetchIgnoreEntries,
		addIgnoreEntry,
		updateIgnoreEntry,
		deleteIgnoreEntry,
		isIgnored,
	};
});
