import { join } from "path";
import { runtimeData } from "./config";
import { existsSync, readFileSync, statSync, writeFileSync } from "fs";
import { getServers, getSites } from "./rest";
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
import { version } from "zod/v4/core";

export function readJSONCache(filename: string, defaultValue: object = []) {
  const filePath = getJSONFilename(filename);

  // If file does not exist, return default value
  if (!existsSync(filePath)) {
    return defaultValue;
  }

  try {
    // If file is more that 24 hours, return default value
    const fileStats = statSync(filePath);
    const fileAge = Date.now() - fileStats.mtimeMs;
    if (fileAge > 86400000) {
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
