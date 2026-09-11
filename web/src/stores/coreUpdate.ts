import { defineStore } from "pinia";
import { useAuthStore } from "./auth";
import { useDataStore } from "./data";
import { useToastStore } from "./toast";
import type { CoreUpdateResult, SiteCore } from "../types";
import { BASE_URL } from "../utils/api";

export const useCoreUpdateStore = defineStore("coreUpdate", () => {
	const authStore = useAuthStore();
	const dataStore = useDataStore();
	const toastStore = useToastStore();

	async function checkCoreUpdate(siteId: number): Promise<SiteCore> {
		const res = await fetch(`${BASE_URL}/sites/${siteId}/core-update`, {
			headers: authStore.authHeader,
		});
		if (!res.ok) {
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

		const core: SiteCore = await res.json();
		dataStore.applyCoreUpdate(siteId, core);
		return core;
	}

	async function updateCore(
		siteId: number,
		target: "minor" | "major",
	): Promise<CoreUpdateResult> {
		const res = await fetch(`${BASE_URL}/sites/${siteId}/core-update`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify({ target }),
		});

		if (!res.ok) {
			if (res.status === 401) {
				authStore.logout();
				throw new Error("Unauthorized");
			}

			let data: any;
			try {
				data = await res.json();
			} catch {
				throw new Error(`Request failed (${res.status})`);
			}

			const site = dataStore.getSiteById(siteId);
			const siteName = site ? site.domain : `Site #${siteId}`;
			let errorMessage = data.error || "Unknown error";
			if (errorMessage.length > 150) {
				errorMessage = errorMessage.substring(0, 147) + "...";
			}

			toastStore.addToast(
				`Failed to update WordPress core on ${siteName}: ${errorMessage}`,
				"error",
				10000,
			);

			const error = new Error(errorMessage);
			(error as any).data = data;
			throw error;
		}

		const result: CoreUpdateResult = await res.json();
		if (result.core) {
			dataStore.applyCoreUpdate(siteId, result.core);
		}

		return result;
	}

	return { checkCoreUpdate, updateCore };
});
