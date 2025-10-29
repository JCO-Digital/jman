import { getCachedServers, getCachedSites } from "./cache";
import { Server } from "./types/server";
import { type CliSite, cliSiteSchema, type Site } from "./types/site";

async function getSiteList(): Promise<CliSite[]> {
  const serverMap = {};
  const sites: CliSite[] = [];
  const cachedServers = await getCachedServers();
  cachedServers.forEach((server: Server) => {
    serverMap[server.id] = server.name;
  });

  const cachedSites = await getCachedSites();
  cachedSites.forEach((site: Site) => {
    const newSite = cliSiteSchema.parse({
      id: site.id,
      site: site.domain,
      serverId: site.server_id,
      serverName: serverMap[site.server_id],
      ssh: `${site.site_user}@${serverMap[site.server_id]}`,
      path: "files",
    });

    sites.push(newSite);
  });

  return sites;
}
