import { ref } from "vue";
import { defineStore } from "pinia";
import type { Server, Site, Plugin } from "../types";

const CACHE_KEY_SERVERS = "jman_servers";
const CACHE_KEY_SITES = "jman_sites";
const CACHE_KEY_PLUGINS = "jman_plugins";
const BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api";

export const useDataStore = defineStore("data", () => {
	// State
	const servers = ref<Server[]>([]);
	const sites = ref<Site[]>([]);
	const plugins = ref<Plugin[]>([]);
	const isLoaded = ref(false);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	// Actions
	function loadFromCache(): boolean {
		try {
			const cachedServers = sessionStorage.getItem(CACHE_KEY_SERVERS);
			const cachedSites = sessionStorage.getItem(CACHE_KEY_SITES);
			const cachedPlugins = sessionStorage.getItem(CACHE_KEY_PLUGINS);

			if (cachedServers && cachedSites && cachedPlugins) {
				servers.value = JSON.parse(cachedServers);
				sites.value = JSON.parse(cachedSites);
				plugins.value = JSON.parse(cachedPlugins);
				isLoaded.value = true;
				return true;
			}
		} catch (e) {
			console.error("Failed to parse cached data", e);
		}
		return false;
	}

	async function fetchFromApi() {
		isLoading.value = true;
		error.value = null;
		try {
			const [serversRes, sitesRes, pluginsRes] = await Promise.all([
				fetch(`${BASE_URL}/servers`),
				fetch(`${BASE_URL}/sites`),
				fetch(`${BASE_URL}/plugins`),
			]);

			if (!serversRes.ok || !sitesRes.ok || !pluginsRes.ok) {
				throw new Error("Failed to fetch data from API endpoints");
			}

			const serversData = await serversRes.json();
			const sitesData = await sitesRes.json();
			const pluginsData = await pluginsRes.json();

			servers.value = serversData;
			sites.value = sitesData;
			plugins.value = pluginsData;

			sessionStorage.setItem(CACHE_KEY_SERVERS, JSON.stringify(serversData));
			sessionStorage.setItem(CACHE_KEY_SITES, JSON.stringify(sitesData));
			sessionStorage.setItem(CACHE_KEY_PLUGINS, JSON.stringify(pluginsData));

			isLoaded.value = true;
		} catch (e: any) {
			console.error("API Fetch error:", e);
			error.value = e.message || "An error occurred while fetching data";
		} finally {
			isLoading.value = false;
		}
	}

	async function initData() {
		if (!isLoaded.value && !isLoading.value) {
			const hasCache = loadFromCache();
			if (!hasCache) {
				await fetchFromApi();
			}
		}
	}

	async function refreshData() {
		await fetchFromApi();
	}

	// Getters
	function getSiteById(id: number) {
		return sites.value.find((s) => s.id === id);
	}

	function getServerById(id: number) {
		return servers.value.find((s) => s.id === id);
	}

	function getPluginsBySiteId(siteId: number) {
		return plugins.value.filter((p) => p.site_id === siteId);
	}

	return {
		// State
		servers,
		sites,
		plugins,
		isLoaded,
		isLoading,
		error,
		// Actions
		initData,
		refreshData,
		// Getters
		getSiteById,
		getServerById,
		getPluginsBySiteId,
	};
});
