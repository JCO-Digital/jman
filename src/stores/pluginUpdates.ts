import { defineStore } from "pinia";
import { useAuthStore } from "./auth";
import { useDataStore } from "./data";
import { useToastStore } from "./toast";
import type { Plugin, PluginUpdateResult } from "../types";
import { BASE_URL } from "../utils/api";

export const usePluginUpdatesStore = defineStore("pluginUpdates", () => {
	const authStore = useAuthStore();
	const dataStore = useDataStore();
	const toastStore = useToastStore();

	async function handleErrorResponse(res: Response): Promise<never> {
		if (res.status === 401) {
			authStore.logout();
			throw new Error("Unauthorized");
		}
		let message: string;
		try {
			const data = await res.json();
			message = data.error || `Request failed (${res.status})`;
		} catch {
			message = `Request failed (${res.status})`;
		}
		throw new Error(message);
	}

	async function fetchPluginUpdates(siteId: number): Promise<Plugin[]> {
		const res = await fetch(`${BASE_URL}/sites/${siteId}/plugin-updates`, {
			headers: authStore.authHeader,
		});
		if (!res.ok) await handleErrorResponse(res);
		return res.json();
	}

	async function updatePlugin(
		siteId: number,
		pluginName: string,
	): Promise<PluginUpdateResult | null> {
		const res = await fetch(`${BASE_URL}/sites/${siteId}/plugin-updates`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify({ plugin: pluginName }),
		});

		if (!res.ok) {
			if (res.status === 401) {
				authStore.logout();
				throw new Error("Unauthorized");
			}

			let data;
			try {
				data = await res.json();
			} catch {
				throw new Error(`Request failed (${res.status})`);
			}

			if (data.status === "failed") {
				const site = dataStore.getSiteById(siteId);
				const siteName = site ? site.domain : `Site #${siteId}`;
				let errorMessage = data.error || "Unknown error";

				// Truncate if too long
				if (errorMessage.length > 150) {
					errorMessage = errorMessage.substring(0, 147) + "...";
				}

				toastStore.addToast(
					`Failed to update ${pluginName} on ${siteName}: ${errorMessage}`,
					"error",
					10000,
				);

				const error = new Error(errorMessage);
				(error as any).data = data;
				throw error;
			}

			throw new Error(data.error || `Request failed (${res.status})`);
		}

		const body = await res.json();
		if (Array.isArray(body)) return null;
		const result = body as PluginUpdateResult;
		dataStore.applyPluginUpdate(siteId, pluginName, result.new_version);
		return result;
	}

	return { fetchPluginUpdates, updatePlugin };
});
