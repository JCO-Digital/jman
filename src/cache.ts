import { getServers, getSites } from "./rest";
import type { Server } from "./types/server";
import type { Site } from "./types/site";
import {
  saveServers,
  saveSites,
  getServersFromDb,
  getSitesFromDb,
} from "./database";

export async function getCachedServers(): Promise<Server[]> {
  let servers = getServersFromDb();
  if (!servers.length) {
    servers = await refreshCachedServers();
  }
  return servers;
}

export async function refreshCachedServers(): Promise<Server[]> {
  const servers = await getServers();
  saveServers(servers);
  return servers;
}

export async function getCachedSites(): Promise<Site[]> {
  let sites = getSitesFromDb();
  if (!sites.length) {
    sites = await refreshCachedSites();
  }
  return sites;
}

export async function refreshCachedSites(): Promise<Site[]> {
  const sites = await getSites();
  saveSites(sites);
  return sites;
}
