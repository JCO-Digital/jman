import { getCachedServers, getCachedSites } from "./cache";
import { Server } from "./types/server";
import { type CliSite, cliSiteSchema, type Site } from "./types/site";
import { confirm } from "@topcli/prompts";

async function getSiteList(): Promise<CliSite[]> {
  const serverMap = {};
  const sites: CliSite[] = [];
  const cachedServers = await getCachedServers();
  cachedServers.forEach((server: Server) => {
    serverMap[server.id] = server.name;
  });

  const cachedSites = await getCachedSites();
  cachedSites.forEach((site: Site) => {
    const serverName = serverMap[site.server_id].split(".")[0];
    const newSite = cliSiteSchema.parse({
      id: site.id,
      name: site.domain,
      serverId: site.server_id,
      serverName,
      ssh: `${site.site_user}@${serverMap[site.server_id]}`,
      path: "files",
    });

    sites.push(newSite);
  });

  return sites;
}

export async function searchSites(query: string): Promise<CliSite[]> {
  const sites = await getSiteList();
  return sites.filter(
    (site) => site.name.includes(query) || site.serverName.includes(query),
  );
}

export function promptSearch(query: string): Promise<CliSite[]> {
  return new Promise((resolve, reject) => {
    if (!query) {
      reject("No query provided");
    }
    searchSites(query).then((sites) => {
      if (sites.length === 0) {
        reject("No sites found");
      } else {
        console.log("Found sites:");
      }
      sites.forEach((site) => {
        console.log(`${site.name} (${site.serverName})`);
      });
      confirm("Do you want to continue?", { initial: true })
        .then(() => {
          resolve(sites);
        })
        .catch(() => {
          reject("User cancelled");
        });
    });
  });
}
