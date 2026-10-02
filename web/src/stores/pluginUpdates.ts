import { defineStore } from "pinia";
import { useAuthStore } from "./auth";
import type { Plugin } from "../types";
import { BASE_URL } from "../utils/api";

export const usePluginUpdatesStore = defineStore("pluginUpdates", () => {
	const authStore = useAuthStore();

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

	async function fetchPluginUpdates(siteId: string): Promise<Plugin[]> {
		const res = await fetch(`${BASE_URL}/sites/${siteId}/plugin-updates`, {
			headers: authStore.authHeader,
		});
		if (!res.ok) await handleErrorResponse(res);
		return res.json();
	}

	return { fetchPluginUpdates };
});
