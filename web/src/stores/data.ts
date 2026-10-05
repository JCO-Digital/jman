import { ref, computed } from "vue";
import { defineStore } from "pinia";
import type {
	Server,
	Site,
	SiteEnvironment,
	Plugin,
	PluginInfo,
	PluginVulnerability,
	CoreVulnReport,
	EnrichedVulnerability,
	EnrichedCoreVulnerability,
	EnrichedSite,
	EnrichedPlugin,
	SiteCore,
} from "../types";
import { useAuthStore } from "./auth";
import { useMonitorStore } from "./monitor";
import { BASE_URL, handleErrorResponse } from "../utils/api";

// Cache keys are versioned: bump a key when the shape of what it holds
// changes (ids moving from SpinupWP integers to UUID strings, sites gaining
// `ssh`), so stale session caches are ignored instead of served.
const CACHE_KEY_SERVERS = "jman_servers_v2";
const CACHE_KEY_SITES = "jman_sites_v3";
const CACHE_KEY_PLUGINS = "jman_plugins_v2";
const CACHE_KEY_PLUGIN_INFO = "jman_plugin_info";
const CACHE_KEY_VULNS = "jman_vulns_v3";
const CACHE_KEY_CORE_VULNS = "jman_core_vulns_v2";
const CACHE_KEY_SITE_ORG_LINKS = "jman_site_organization_links_v2";
const LEGACY_CACHE_KEYS = [
	"jman_servers",
	"jman_sites",
	"jman_sites_v2",
	"jman_plugins",
	"jman_vulns_v2",
	"jman_core_vulns",
	"jman_site_organization_links",
];

/** Display label for a site's server, e.g. for lists and detail views. */
export const NO_SERVER_LABEL = "No server";

export const useDataStore = defineStore("data", () => {
	// State
	const servers = ref<Server[]>([]);
	const sites = ref<Site[]>([]);
	const siteOrganizationLinks = ref<Record<string, number>>({});
	const plugins = ref<Plugin[]>([]);
	const pluginInfo = ref<PluginInfo[]>([]);
	const vulnerabilities = ref<PluginVulnerability[]>([]);
	const coreVulnerabilities = ref<CoreVulnReport[]>([]);

	const isLoaded = ref(false);
	const isLoading = ref(false);
	const isVulnsLoading = ref(false);
	const error = ref<string | null>(null);
	const vulnsError = ref<string | null>(null);

	// Optimization Maps
	const vulnerabilitiesBySlug = computed(() => {
		const map = new Map<string, EnrichedVulnerability[]>();
		for (const pv of vulnerabilities.value) {
			map.set(
				pv.slug,
				pv.vulnerabilities.map((v) => ({
					...v,
					slug: pv.slug,
					plugin_name: pv.plugin_name,
					plugin_suppressed: pv.suppressed,
				})),
			);
		}
		return map;
	});

	const vulnerabilitiesBySiteId = computed(() => {
		const map = new Map<string, EnrichedVulnerability[]>();
		for (const pv of vulnerabilities.value) {
			for (const v of pv.vulnerabilities) {
				for (const s of v.sites) {
					if (!map.has(s.site_id)) map.set(s.site_id, []);
					map.get(s.site_id)!.push({
						...v,
						slug: pv.slug,
						plugin_name: pv.plugin_name,
						plugin_suppressed: pv.suppressed,
					});
				}
			}
		}
		return map;
	});

	// Core vulnerabilities are reported per core version rather than per plugin,
	// so they are keyed back to the sites running that version.
	const coreVulnerabilitiesBySiteId = computed(() => {
		const map = new Map<string, EnrichedCoreVulnerability[]>();
		for (const report of coreVulnerabilities.value) {
			if (report.suppressed) continue;

			for (const v of report.vulnerabilities) {
				for (const s of v.sites) {
					if (!map.has(s.site_id)) map.set(s.site_id, []);
					map.get(s.site_id)!.push({
						...v,
						core_version: report.version,
						// A core vulnerability is effectively suppressed for a
						// site if the vulnerability itself is ignored or the
						// site/server is ignored.
						suppressed: v.suppressed || s.suppressed,
					});
				}
			}
		}
		return map;
	});

	const pluginsBySiteIdMap = computed(() => {
		const map = new Map<string, Plugin[]>();
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

	const pluginNameMap = computed(() => {
		const map = new Map<string, string>();
		for (const info of enrichedPlugins.value) {
			map.set(info.slug, info.name);
		}
		return map;
	});

	const sitesByIdMap = computed(() => {
		const map = new Map<string, Site>();
		for (const s of sites.value) {
			map.set(s.id, s);
		}
		return map;
	});

	const serversByIdMap = computed(() => {
		const map = new Map<string, Server>();
		for (const s of servers.value) {
			map.set(s.id, s);
		}
		return map;
	});

	const activeVulnerabilities = computed(() => {
		// Filter out vulnerabilities that are suppressed at the plugin level
		// and only count sites where the vulnerability is not suppressed at the site/server level.
		const active: EnrichedVulnerability[] = [];
		for (const pv of vulnerabilities.value) {
			if (pv.suppressed) continue;

			for (const v of pv.vulnerabilities) {
				if (v.suppressed) continue;

				const activeSites = v.sites.filter((s) => !s.suppressed);
				if (activeSites.length > 0) {
					active.push({
						...v,
						slug: pv.slug,
						plugin_name: pv.plugin_name,
						plugin_suppressed: pv.suppressed,
						sites: activeSites,
					});
				}
			}
		}
		return active;
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

			const vulns = (
				vulnerabilitiesBySlug.value.get(info.slug) || []
			).map((v) => ({
				...v,
				// Effective suppression at the plugin level:
				// either the plugin is suppressed OR the specific vulnerability is suppressed.
				suppressed: v.plugin_suppressed || v.suppressed,
			}));

			return {
				...info,
				name,
				shortName,
				version: info.version || "N/A",
				author: info.author || "Unknown",
				count,
				vulnerabilities: vulns,
			};
		});
	});

	const enrichedSites = computed<EnrichedSite[]>(() => {
		const monitorStore = useMonitorStore();
		return sites.value.map((site) => {
			const vulns = (
				vulnerabilitiesBySiteId.value.get(site.id) || []
			).map((v) => {
				// Find the entry for THIS site in the vulnerability report
				const siteSpecificVuln = v.sites.find(
					(s) => s.site_id === site.id,
				);
				return {
					...v,
					// A vulnerability is effectively suppressed for a site if:
					// 1. The plugin is ignored globally (v.plugin_suppressed)
					// 2. The vulnerability itself is ignored (v.suppressed)
					// 3. The site/server is ignored (siteSpecificVuln.suppressed)
					suppressed:
						v.plugin_suppressed ||
						v.suppressed ||
						siteSpecificVuln?.suppressed ||
						false,
				};
			});

			return {
				...site,
				organization_id:
					site.organization_id ??
					siteOrganizationLinks.value[site.id],
				server: serverNameForSite(site),
				plugins: pluginsBySiteIdMap.value.get(site.id) || [],
				vulnerabilities: vulns,
				coreVulnerabilities:
					coreVulnerabilitiesBySiteId.value.get(site.id) || [],
				monitorHistory:
					monitorStore.historyByDomain.get(site.domain) || [],
			};
		});
	});

	function serverNameForSite(site: Site): string {
		if (site.server_name) return site.server_name;
		if (site.server_id == null) return NO_SERVER_LABEL;
		return serversByIdMap.value.get(site.server_id)?.name ?? "—";
	}

	// Actions
	function loadFromCache(): boolean {
		try {
			for (const key of LEGACY_CACHE_KEYS) sessionStorage.removeItem(key);
		} catch {
			// ignore storage access errors
		}
		try {
			const cachedServers = sessionStorage.getItem(CACHE_KEY_SERVERS);
			const cachedSites = sessionStorage.getItem(CACHE_KEY_SITES);
			const cachedPlugins = sessionStorage.getItem(CACHE_KEY_PLUGINS);
			const cachedPluginInfo = sessionStorage.getItem(
				CACHE_KEY_PLUGIN_INFO,
			);
			const cachedVulns = sessionStorage.getItem(CACHE_KEY_VULNS);
			const cachedCoreVulns =
				sessionStorage.getItem(CACHE_KEY_CORE_VULNS);

			const cachedLinks = sessionStorage.getItem(
				CACHE_KEY_SITE_ORG_LINKS,
			);

			if (
				cachedServers &&
				cachedSites &&
				cachedPlugins &&
				cachedPluginInfo
			) {
				servers.value = JSON.parse(cachedServers);
				sites.value = JSON.parse(cachedSites);
				if (cachedLinks) {
					siteOrganizationLinks.value = JSON.parse(cachedLinks);
				}
				plugins.value = JSON.parse(cachedPlugins);
				pluginInfo.value = JSON.parse(cachedPluginInfo);
				if (cachedVulns) {
					vulnerabilities.value = JSON.parse(cachedVulns);
				}
				if (cachedCoreVulns) {
					coreVulnerabilities.value = JSON.parse(cachedCoreVulns);
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
		sessionStorage.removeItem(CACHE_KEY_SITE_ORG_LINKS);
		sessionStorage.removeItem(CACHE_KEY_PLUGINS);
		sessionStorage.removeItem(CACHE_KEY_PLUGIN_INFO);
		sessionStorage.removeItem(CACHE_KEY_VULNS);
		sessionStorage.removeItem(CACHE_KEY_CORE_VULNS);
		servers.value = [];
		sites.value = [];
		siteOrganizationLinks.value = {};
		plugins.value = [];
		pluginInfo.value = [];
		vulnerabilities.value = [];
		coreVulnerabilities.value = [];
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
			vulnsError.value = null;
			fetch(`${BASE_URL}/vulns/core`, { headers })
				.then(async (res) => {
					if (res.ok) {
						const data = await res.json();
						coreVulnerabilities.value = data;
						sessionStorage.setItem(
							CACHE_KEY_CORE_VULNS,
							JSON.stringify(data),
						);
					} else if (res.status !== 401) {
						console.error(
							"Failed to fetch core vulnerabilities:",
							res.statusText,
						);
					}
				})
				.catch((err) => {
					console.error("Failed to fetch core vulnerabilities:", err);
				});

			fetch(`${BASE_URL}/vulns`, { headers })
				.then(async (res) => {
					if (res.ok) {
						const data = await res.json();
						vulnerabilities.value = data;
						sessionStorage.setItem(
							CACHE_KEY_VULNS,
							JSON.stringify(data),
						);
					} else if (res.status !== 401) {
						vulnsError.value = "Failed to load vulnerability data";
						console.error(
							"Failed to fetch vulnerabilities:",
							res.statusText,
						);
					}
				})
				.catch((err) => {
					vulnsError.value = "Failed to load vulnerability data";
					console.error("Failed to fetch vulnerabilities:", err);
				})
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

			sessionStorage.setItem(
				CACHE_KEY_SERVERS,
				JSON.stringify(serversData),
			);
			sessionStorage.setItem(CACHE_KEY_SITES, JSON.stringify(sitesData));
			sessionStorage.setItem(
				CACHE_KEY_PLUGINS,
				JSON.stringify(pluginsData),
			);
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

	function getSiteById(id: string | null | undefined) {
		if (id == null) return undefined;
		return sitesByIdMap.value.get(id);
	}

	function getServerById(id: string | null | undefined) {
		if (id == null) return undefined;
		return serversByIdMap.value.get(id);
	}

	function getPluginsBySiteId(siteId: string) {
		return pluginsBySiteIdMap.value.get(siteId) || [];
	}

	function getVulnerabilitiesBySlug(slug: string) {
		return vulnerabilitiesBySlug.value.get(slug) || [];
	}

	function applyPluginUpdate(
		siteId: string,
		pluginName: string,
		newVersion: string,
	) {
		const plugin = plugins.value.find(
			(p) => p.site_id === siteId && p.name === pluginName,
		);
		if (plugin) {
			plugin.version = newVersion;
			plugin.update = "";

			// Persist to session storage so it survives reloads
			sessionStorage.setItem(
				CACHE_KEY_PLUGINS,
				JSON.stringify(plugins.value),
			);
		}
	}

	/**
	 * Reloads one site's plugins from jman-api's cache, e.g. after a
	 * background plugin change or an update check refreshed it.
	 */
	async function reloadSitePlugins(siteId: string) {
		const authStore = useAuthStore();
		const res = await fetch(`${BASE_URL}/sites/${siteId}/plugins`, {
			headers: authStore.authHeader,
		});
		if (!res.ok) await handleErrorResponse(res);
		const sitePlugins: Plugin[] = await res.json();
		plugins.value = [
			...plugins.value.filter((p) => p.site_id !== siteId),
			...sitePlugins,
		];
		sessionStorage.setItem(
			CACHE_KEY_PLUGINS,
			JSON.stringify(plugins.value),
		);
	}

	function applyCoreUpdate(siteId: string, core: SiteCore) {
		const site = sites.value.find((s) => s.id === siteId);
		if (site) {
			site.wp_core = core;

			// Persist to session storage so it survives reloads
			sessionStorage.setItem(
				CACHE_KEY_SITES,
				JSON.stringify(sites.value),
			);
		}
	}

	async function setSiteEnvironment(
		siteId: string,
		environment: SiteEnvironment | "",
	) {
		const authStore = useAuthStore();

		const res = await fetch(`${BASE_URL}/sites/${siteId}/environment`, {
			method: "PATCH",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify({ environment }),
		});
		if (!res.ok) await handleErrorResponse(res);

		const site = sites.value.find((s) => s.id === siteId);
		if (site) {
			site.environment = environment || undefined;
			sessionStorage.setItem(
				CACHE_KEY_SITES,
				JSON.stringify(sites.value),
			);
		}
	}

	function setSiteOrganizationLink(
		siteId: string,
		organizationId: number | undefined,
	) {
		if (organizationId === undefined) {
			delete siteOrganizationLinks.value[siteId];
		} else {
			siteOrganizationLinks.value[siteId] = organizationId;
		}
		sessionStorage.setItem(
			CACHE_KEY_SITE_ORG_LINKS,
			JSON.stringify(siteOrganizationLinks.value),
		);
	}

	return {
		// State
		servers,
		sites,
		plugins,
		pluginInfo,
		vulnerabilities,
		coreVulnerabilities,
		activeVulnerabilities,
		isLoaded,
		isLoading,
		isVulnsLoading,
		error,
		vulnsError,
		// Getters
		enrichedPlugins,
		enrichedSites,
		vulnerabilitiesBySlug,
		vulnerabilitiesBySiteId,
		coreVulnerabilitiesBySiteId,
		pluginsBySiteIdMap,
		pluginsBySlugMap,
		pluginNameMap,
		sitesByIdMap,
		serversByIdMap,
		getSiteById,
		getServerById,
		serverNameForSite,
		getPluginsBySiteId,
		getVulnerabilitiesBySlug,
		setSiteOrganizationLink,
		setSiteEnvironment,
		applyPluginUpdate,
		applyCoreUpdate,
		reloadSitePlugins,
		// Actions
		initData,
		refreshData,
		clearCache,
	};
});
