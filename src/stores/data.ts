import { ref, computed } from "vue";
import { defineStore } from "pinia";
import type { Server, Site, Plugin, PluginInfo } from "../types";
import { useAuthStore } from "./auth";

const CACHE_KEY_SERVERS = "jman_servers";
const CACHE_KEY_SITES = "jman_sites";
const CACHE_KEY_PLUGINS = "jman_plugins";
const CACHE_KEY_PLUGIN_INFO = "jman_plugin_info";
const BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api";

export const useDataStore = defineStore("data", () => {
	// State
	const servers = ref<Server[]>([]);
	const sites = ref<Site[]>([]);
	const plugins = ref<Plugin[]>([]);
	const pluginInfo = ref<PluginInfo[]>([]);

	const isLoaded = ref(false);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	// Getters
	const enrichedPlugins = computed(() => {
		return pluginInfo.value.map((info) => {
			const count = plugins.value.filter((p) => p.name === info.slug).length;
			let name = info.name || info.slug || "Unknown Plugin";
			if (name === info.slug) {
				// Turn "advanced-custom-fields-pro" into "Advanced Custom Fields Pro".
				name = name
					.split(/[-_]/)
					.map((word) => word.charAt(0).toUpperCase() + word.slice(1))
					.join(" ");
			}
			let shortName = name;

			if (shortName) {
				const parts = shortName.split(/ [-–—] /);
				if (parts.length > 1 && parts[0] !== undefined) {
					shortName = parts[0];
				}
			}

			return {
				...info,
				name,
				shortName,
				version: info.version || "N/A",
				author: info.author || "Unknown",
				count,
			};
		});
	});

	const enrichedSites = computed(() => {
		return sites.value.map((site) => {
			return {
				...site,
				server: servers.value.find((s) => s.id === site.server_id),
				plugins: plugins.value.filter((p) => p.site_id === site.id),
			};
		});
	});

	// Actions
	function loadFromCache(): boolean {
		try {
			const cachedServers = sessionStorage.getItem(CACHE_KEY_SERVERS);
			const cachedSites = sessionStorage.getItem(CACHE_KEY_SITES);
			const cachedPlugins = sessionStorage.getItem(CACHE_KEY_PLUGINS);
			const cachedPluginInfo = sessionStorage.getItem(CACHE_KEY_PLUGIN_INFO);

			if (cachedServers && cachedSites && cachedPlugins && cachedPluginInfo) {
				servers.value = JSON.parse(cachedServers);
				sites.value = JSON.parse(cachedSites);
				plugins.value = JSON.parse(cachedPlugins);
				pluginInfo.value = JSON.parse(cachedPluginInfo);
				isLoaded.value = true;
				return true;
			}
		} catch (e) {
			console.error("Failed to parse cached data", e);
		}
		return false;
	}

	function clearCache() {
		sessionStorage.removeItem(CACHE_KEY_SERVERS);
		sessionStorage.removeItem(CACHE_KEY_SITES);
		sessionStorage.removeItem(CACHE_KEY_PLUGINS);
		sessionStorage.removeItem(CACHE_KEY_PLUGIN_INFO);
		servers.value = [];
		sites.value = [];
		plugins.value = [];
		pluginInfo.value = [];
		isLoaded.value = false;
	}

	async function fetchFromApi() {
		const authStore = useAuthStore();

		isLoading.value = true;
		error.value = null;
		try {
			const headers: Record<string, string> = {
				...authStore.authHeader,
			};

			const [serversRes, sitesRes, pluginsRes, pluginInfoRes] =
				await Promise.all([
					fetch(`${BASE_URL}/servers`, { headers }),
					fetch(`${BASE_URL}/sites`, { headers }),
					fetch(`${BASE_URL}/plugins`, { headers }),
					fetch(`${BASE_URL}/plugininfo`, { headers }),
				]);

			// Handle 401 on any response — token is invalid or expired
			if (
				serversRes.status === 401 ||
				sitesRes.status === 401 ||
				pluginsRes.status === 401 ||
				pluginInfoRes.status === 401
			) {
				clearCache();
				authStore.logout();
				return;
			}

			if (
				!serversRes.ok ||
				!sitesRes.ok ||
				!pluginsRes.ok ||
				!pluginInfoRes.ok
			) {
				throw new Error("Failed to fetch data from API endpoints");
			}

			const serversData = await serversRes.json();
			const sitesData = await sitesRes.json();
			const pluginsData = await pluginsRes.json();
			const pluginInfoData: PluginInfo[] = await pluginInfoRes.json();

			servers.value = serversData;
			sites.value = sitesData;
			plugins.value = pluginsData;
			pluginInfo.value = pluginInfoData;

			sessionStorage.setItem(CACHE_KEY_SERVERS, JSON.stringify(serversData));
			sessionStorage.setItem(CACHE_KEY_SITES, JSON.stringify(sitesData));
			sessionStorage.setItem(CACHE_KEY_PLUGINS, JSON.stringify(pluginsData));
			sessionStorage.setItem(
				CACHE_KEY_PLUGIN_INFO,
				JSON.stringify(pluginInfoData),
			);

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
		pluginInfo,
		isLoaded,
		isLoading,
		error,
		// Getters
		enrichedPlugins,
		enrichedSites,
		getSiteById,
		getServerById,
		getPluginsBySiteId,
		// Actions
		initData,
		refreshData,
		clearCache,
	};
});
