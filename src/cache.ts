import { dirname, join } from "path";
import { runtimeData } from "./config";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  statSync,
  writeFileSync,
} from "fs";
import { getServers, getSites, getWpVulnerabilities } from "./rest";
import { getPlugins } from "./wp-cli";
import { getSiteList } from "./search";
import type { Server } from "./types/server";
import type { Site } from "./types/site";
import {
  pluginDataSchema,
  pluginSiteSchema,
  type WpPlugin,
  type WpPluginData,
} from "./types/plugin";
import { vulnResponseSchema } from "./types/vuln";

export function readJSONCache(filename: string, defaultValue: object = []) {
  const filePath = getJSONFilename(filename);

  // If file does not exist, return default value
  if (!existsSync(filePath)) {
    return defaultValue;
  }

  try {
    // If file is more than 12 hours, return default value
    const fileStats = statSync(filePath);
    const fileAge = Date.now() - fileStats.mtimeMs;
    if (fileAge > 43200000) {
      return defaultValue;
    }

    return JSON.parse(readFileSync(filePath, "utf8"));
  } catch (error) {
    console.error(error);
    return defaultValue;
  }
}

export function writeJSONCache(filename: string, data: object) {
  const filePath = getJSONFilename(filename);

  // If folder does not exist, create it
  const folderPath = dirname(filePath);
  if (!existsSync(folderPath)) {
    mkdirSync(folderPath, { recursive: true });
  }

  try {
    writeFileSync(filePath, JSON.stringify(data, null, 2), "utf8");
  } catch (error) {
    console.error(`Failed to write cache file ${filename}:`, error);
  }
}

function getJSONFilename(filename: string): string {
  return join(runtimeData.cacheDir, `${filename}.json`);
}

export async function getCachedServers() {
  let servers = readJSONCache("servers");
  if (!servers.length) {
    servers = refreshCachedServers();
  }
  return servers;
}

export async function refreshCachedServers(): Promise<Server[]> {
  const servers = await getServers();
  writeJSONCache("servers", servers);
  return servers;
}

export async function getCachedSites() {
  let sites = readJSONCache("sites");
  if (!sites.length) {
    sites = refreshCachedSites();
  }
  return sites;
}

export async function refreshCachedSites(): Promise<Site[]> {
  const sites = await getSites();
  writeJSONCache("sites", sites);
  return sites;
}

export async function getCachedVulnerabilities(plugin: string) {
  const filename = join("vulnerabilities", plugin);
  const data = readJSONCache(filename, {});

  const result = vulnResponseSchema.safeParse(data);
  if (result.success) {
    let vulnerabilities = result.data;
    if (vulnerabilities.error !== 0) {
      console.error(`Fetching vulnerabilities for ${plugin}`);
      vulnerabilities = await getWpVulnerabilities(plugin);
      writeJSONCache(filename, vulnerabilities);
    }

    return vulnerabilities;
  } else {
    console.error(
      `Error parsing vulnerabilities for ${plugin}: ${result.error}`,
    );
    console.error(data);
  }
}

export async function getCachedPlugins() {
  const plugins: WpPlugin[] = readJSONCache("plugins");

  const sites = await getSiteList();
  for (const site of sites) {
    if (plugins.filter((plugin) => plugin.site_id === site.id)) {
      continue;
    }

    for (const plugin of await getPlugins(site)) {
      plugins.push(plugin);
    }
  }

  writeJSONCache("plugins", plugins);
  return plugins;
}

export async function getCachedPluginData() {
  const pluginData: WpPluginData[] = [];

  for (const plugin of await getCachedPlugins()) {
    if (plugin.status !== "active") {
      continue;
    }
    let found = false;
    for (const item of pluginData) {
      if (item.name === plugin.name) {
        found = true;
        item.sites.push(pluginSiteSchema.parse(plugin));
      }
    }
    if (!found) {
      pluginData.push(
        pluginDataSchema.parse({
          name: plugin.name,
          sites: [{ site_id: plugin.site_id, version: plugin.version }],
        }),
      );
    }
  }

  return pluginData;
}
