import { join } from "path";
import { runtimeData } from "./config";
import { readFileSync, writeFileSync } from "fs";
import { getServers, getSites } from "./rest";

export function readJSONCache(filename: string, defaultValue: object = []) {
  const filePath = getJSONFilename(filename);
  try {
    return JSON.parse(readFileSync(filePath, "utf8"));
  } catch (error) {
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

function getJSONFilename(filename: string) {
  return join(runtimeData.cacheDir, `${filename}.json`);
}

export async function getCachedServers() {
  let servers = readJSONCache("servers");
  if (!servers.length) {
    servers = await getServers();
    console.log(servers);
    writeJSONCache("servers", servers);
  }
  return servers;
}

export async function getCachedSites() {
  let sites = readJSONCache("sites");
  if (!sites.length) {
    sites = await getSites();
    writeJSONCache("sites", sites);
  }
  return sites;
}
