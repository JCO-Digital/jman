import { defineStore } from "pinia";
import { useAuthStore } from "./auth";
import { useDataStore } from "./data";
import { useToastStore } from "./toast";
import { useUpdateJobsStore } from "./updateJobs";
import type { SiteCore, UpdateJob } from "../types";
import { BASE_URL } from "../utils/api";

export const useCoreUpdateStore = defineStore("coreUpdate", () => {
	const authStore = useAuthStore();
	const dataStore = useDataStore();
	const toastStore = useToastStore();
	const jobsStore = useUpdateJobsStore();

	async function checkCoreUpdate(siteId: string): Promise<SiteCore> {
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

	/**
	 * Queues a background core update. Progress and the outcome show in the
	 * task center; the job store applies the new version when it finishes.
	 */
	async function updateCore(
		siteId: string,
		target: "minor" | "major",
	): Promise<UpdateJob> {
		try {
			return await jobsStore.enqueueCore(siteId, target);
		} catch (e: any) {
			const site = dataStore.getSiteById(siteId);
			const siteName = site ? site.domain : `Site #${siteId}`;
			toastStore.addToast(
				`Failed to queue WordPress core update on ${siteName}: ${e.message}`,
				"error",
				10000,
			);
			throw e;
		}
	}

	return { checkCoreUpdate, updateCore };
});
