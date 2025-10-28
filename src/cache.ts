import { join } from "path";
import { runtimeData } from "./config";
import { readFileSync, writeFileSync } from "fs";
import { getServers, getSites } from "./rest";
import type { Server } from "./types/server";
import type { Site } from "./types/site";

export function readJSONCache(filename: string, defaultValue: object = []) {
  const filePath = getJSONFilename(filename);
  try {
    return JSON.parse(readFileSync(filePath, "utf8"));
  } catch (error) {
    console.error(error);
    return defaultValue;
  }
}

export function writeJSONCache(filename: string, data: object) {
  const filePath = getJSONFilename(filename);
  console.log(filePath);
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
