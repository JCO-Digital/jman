import { defineStore } from "pinia";
import { ref } from "vue";
import { useDataStore } from "./data";
import type { Plugin } from "../types";
import { BASE_URL, handleErrorResponse, apiFetch } from "../utils/api";

/** How many sites are checked at once when checking several. */
const CHECK_CONCURRENCY = 4;

/**
 * Live update checks. jman-api asks the site over WP-CLI, refreshes its
 * cached plugin list, and the data store then reloads that site's plugins,
 * so the plugin tables show the fresh update state.
 */
export const usePluginUpdatesStore = defineStore("pluginUpdates", () => {
	const dataStore = useDataStore();

	/** Site IDs with a check in flight. */
	const checking = ref(new Set<string>());

	function isChecking(siteId: string): boolean {
		return checking.value.has(siteId);
	}

	/** Checks one site; returns the plugins with an update available. */
	async function checkSite(siteId: string): Promise<Plugin[]> {
		checking.value = new Set(checking.value).add(siteId);
		try {
			const res = await apiFetch(
				`${BASE_URL}/sites/${siteId}/plugin-updates`,
			);
			if (!res.ok) await handleErrorResponse(res);
			const updates: Plugin[] = await res.json();
			await dataStore.reloadSitePlugins(siteId);
			return updates;
		} finally {
			const next = new Set(checking.value);
			next.delete(siteId);
			checking.value = next;
		}
	}

	/**
	 * Checks several sites, a few at a time. onProgress is called after each
	 * site; failures are collected rather than stopping the run.
	 */
	async function checkSites(
		siteIds: string[],
		onProgress?: (done: number, total: number) => void,
	): Promise<{ failed: { siteId: string; error: string }[] }> {
		const failed: { siteId: string; error: string }[] = [];
		let next = 0;
		let done = 0;
		const worker = async () => {
			while (next < siteIds.length) {
				const siteId = siteIds[next++]!;
				try {
					await checkSite(siteId);
				} catch (e: any) {
					failed.push({ siteId, error: e.message || "Check failed" });
				}
				onProgress?.(++done, siteIds.length);
			}
		};
		await Promise.all(
			Array.from(
				{ length: Math.min(CHECK_CONCURRENCY, siteIds.length) },
				worker,
			),
		);
		return { failed };
	}

	return { isChecking, checkSite, checkSites };
});
