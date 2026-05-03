import { ref, computed } from "vue";
import { defineStore } from "pinia";
import type {
	Server,
	Site,
	Plugin,
	PluginInfo,
	Vulnerability,
	EnrichedSite,
	EnrichedPlugin,
} from "../types";
import { useAuthStore } from "./auth";
import { useMonitorStore } from "./monitor";

const CACHE_KEY_SERVERS = "jman_servers";
const CACHE_KEY_SITES = "jman_sites";
const CACHE_KEY_PLUGINS = "jman_plugins";
const CACHE_KEY_PLUGIN_INFO = "jman_plugin_info";
const CACHE_KEY_VULNS = "jman_vulns";
const BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api";

export const useDataStore = defineStore("data", () => {
	// State
	const servers = ref<Server[]>([]);
	const sites = ref<Site[]>([]);
	const siteCompanyLinks = ref<Record<number, number>>({});
	const plugins = ref<Plugin[]>([]);
	const pluginInfo = ref<PluginInfo[]>([]);
	const vulnerabilities = ref<Vulnerability[]>([]);

	const isLoaded = ref(false);
	const isLoading = ref(false);
	const isVulnsLoading = ref(false);
	const error = ref<string | null>(null);

	// Optimization Maps
	const vulnerabilitiesBySlug = computed(() => {
		const map = new Map<string, Vulnerability[]>();
		for (const v of vulnerabilities.value) {
			if (!map.has(v.slug)) map.set(v.slug, []);
			map.get(v.slug)!.push(v);
		}
		return map;
	});

	const vulnerabilitiesBySiteId = computed(() => {
		const map = new Map<number, Vulnerability[]>();
		for (const v of vulnerabilities.value) {
			for (const s of v.sites) {
				if (!map.has(s.site_id)) map.set(s.site_id, []);
				map.get(s.site_id)!.push(v);
			}
		}
		return map;
	});

	const pluginsBySiteIdMap = computed(() => {
		const map = new Map<number, Plugin[]>();
		for (const p of plugins.value) {
			if (!map.has(p.site_id)) map.set(p.site_id, []);
			map.get(p.site_id)!.push(p);
		}
		return map;
	});

	const pluginsBySlugMap = computed(() => {
		const map = new Map<string, Plugin[]>();
		for (const p of plugins.value) {
			if (!map.has(p.name)) map.set(p.name, []);
			map.get(p.name)!.push(p);
		}
		return map;
	});

	const sitesByIdMap = computed(() => {
		const map = new Map<number, Site>();
		for (const s of sites.value) {
			map.set(s.id, s);
		}
		return map;
	});

	const serversByIdMap = computed(() => {
		const map = new Map<number, Server>();
		for (const s of servers.value) {
			map.set(s.id, s);
		}
		return map;
	});

	// Getters
	const enrichedPlugins = computed<EnrichedPlugin[]>(() => {
		return pluginInfo.value.map((info) => {
			const count = pluginsBySlugMap.value.get(info.slug)?.length || 0;
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
				vulnerabilities: vulnerabilitiesBySlug.value.get(info.slug) || [],
			};
		});
	});

	const enrichedSites = computed<EnrichedSite[]>(() => {
		const monitorStore = useMonitorStore();
		return sites.value.map((site) => {
			return {
				...site,
				company_id: site.company_id ?? siteCompanyLinks.value[site.id],
				server:
					serversByIdMap.value.get(site.server_id)?.name ?? "Unknown Server",
				plugins: pluginsBySiteIdMap.value.get(site.id) || [],
				vulnerabilities: vulnerabilitiesBySiteId.value.get(site.id) || [],
				monitorHistory: monitorStore.historyByDomain.get(site.domain) || [],
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
			const cachedVulns = sessionStorage.getItem(CACHE_KEY_VULNS);

			const cachedLinks = sessionStorage.getItem("jman_site_company_links");

			if (cachedServers && cachedSites && cachedPlugins && cachedPluginInfo) {
				servers.value = JSON.parse(cachedServers);
				sites.value = JSON.parse(cachedSites);
				if (cachedLinks) {
					siteCompanyLinks.value = JSON.parse(cachedLinks);
				}
				plugins.value = JSON.parse(cachedPlugins);
				pluginInfo.value = JSON.parse(cachedPluginInfo);
				if (cachedVulns) {
					vulnerabilities.value = JSON.parse(cachedVulns);
				}
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
		sessionStorage.removeItem("jman_site_company_links");
		sessionStorage.removeItem(CACHE_KEY_PLUGINS);
		sessionStorage.removeItem(CACHE_KEY_PLUGIN_INFO);
		sessionStorage.removeItem(CACHE_KEY_VULNS);
		servers.value = [];
		sites.value = [];
		siteCompanyLinks.value = {};
		plugins.value = [];
		pluginInfo.value = [];
		vulnerabilities.value = [];
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

			const monitorStore = useMonitorStore();

			const [serversRes, sitesRes, pluginsRes, pluginInfoRes] =
				await Promise.all([
					fetch(`${BASE_URL}/servers`, { headers }),
					fetch(`${BASE_URL}/sites`, { headers }),
					fetch(`${BASE_URL}/plugins`, { headers }),
					fetch(`${BASE_URL}/plugininfo`, { headers }),
					monitorStore.fetchHistory(),
				]);

			// Fetch vulnerabilities separately to not block primary data
			isVulnsLoading.value = true;
			fetch(`${BASE_URL}/vulns`, { headers })
				.then(async (res) => {
					if (res.ok) {
						const data = await res.json();
						vulnerabilities.value = data;
						sessionStorage.setItem(CACHE_KEY_VULNS, JSON.stringify(data));
					} else if (res.status !== 401) {
						console.error("Failed to fetch vulnerabilities:", res.statusText);
					}
				})
				.catch((err) => console.error("Failed to fetch vulnerabilities:", err))
				.finally(() => {
					isVulnsLoading.value = false;
				});

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
			} else {
				const monitorStore = useMonitorStore();
				await monitorStore.ensureHistory();
			}
		}
	}

	async function refreshData() {
		await fetchFromApi();
	}

	function getSiteById(id: number) {
		return sitesByIdMap.value.get(id);
	}

	function getServerById(id: number) {
		return serversByIdMap.value.get(id);
	}

	function getPluginsBySiteId(siteId: number) {
		return pluginsBySiteIdMap.value.get(siteId) || [];
	}

	function getVulnerabilitiesBySlug(slug: string) {
		return vulnerabilitiesBySlug.value.get(slug) || [];
	}

	function setSiteCompanyLink(siteId: number, companyId: number | undefined) {
		if (companyId === undefined) {
			delete siteCompanyLinks.value[siteId];
		} else {
			siteCompanyLinks.value[siteId] = companyId;
		}
		sessionStorage.setItem(
			"jman_site_company_links",
			JSON.stringify(siteCompanyLinks.value),
		);
	}

	return {
		// State
		servers,
		sites,
		plugins,
		pluginInfo,
		vulnerabilities,
		isLoaded,
		isLoading,
		isVulnsLoading,
		error,
		// Getters
		enrichedPlugins,
		enrichedSites,
		vulnerabilitiesBySlug,
		vulnerabilitiesBySiteId,
		pluginsBySiteIdMap,
		pluginsBySlugMap,
		sitesByIdMap,
		serversByIdMap,
		getSiteById,
		getServerById,
		getPluginsBySiteId,
		getVulnerabilitiesBySlug,
		setSiteCompanyLink,
		// Actions
		initData,
		refreshData,
		clearCache,
	};
});
