import { refreshCachedServers, refreshCachedSites } from "./cache";
import { runtimeData } from "./config";
import { cmdSchema, type jCmd } from "./types";
import { Site } from "./types/site";
import { Server } from "./types/server";
import { stringify } from "yaml";
import { runWP } from "./wp-cli";
import { promptSearch, searchSites } from "./search";

export function parser(args: string[]): jCmd {
  const cmdData: jCmd = cmdSchema.parse({});

  // Get node bin from start of array.
  runtimeData.nodePath = args.shift() ?? "";
  // Get script path from next arg.
  runtimeData.scriptPath = args.shift() ?? "";
  // Get Command from next arg.
  if (args.length > 0) {
    cmdData.cmd = args.shift() ?? "";
  }
  // Get remaining args as target.
  if (args.length > 0) {
    cmdData.target = args;
  }

  return cmdData;
}

export function runCmd(data: jCmd) {
  switch (data.cmd) {
    case "":
      console.error("No command provided.");
      break;
    case "fetch":
      if (data.target.length === 0) {
        console.error("No target provided for fetch command.");
        console.error("Specify: servers, sites or all.");
      }
      if (data.target.includes("all") || data.target.includes("servers")) {
        refreshCachedServers().then((servers) => {
          console.error("Refreshed servers:", servers.length);
        });
      }
      if (data.target.includes("all") || data.target.includes("sites")) {
        refreshCachedSites().then((sites) => {
          console.error("Refreshed sites:", sites.length);
        });
      }
      break;
    case "list":
      console.error("Not implemented");
      break;
    case "wp":
      runWPCmd(data.target.shift() ?? "", data.target.join(" "));
      break;
    case "search":
      searchSites(data.target.shift() ?? "").then((sites) => {
        console.log("Search results:");
        sites.forEach((site) => {
          console.log(`${site.name} (${site.serverName})`);
        });
      });
      break;
    case "alias":
      createAliases(data.target.shift() ?? "");
      break;
    default:
      console.error("Unknown command.");
  }
}

async function runWPCmd(search: string, command: string) {
  const searchResults = await promptSearch(search);
  for (const result of searchResults) {
    console.log(
      `Running command '${command}' on ${result.name} (${result.serverName})`,
    );
    const ret = await runWP(result.ssh, result.path, command);
    console.log(ret.output);
  }
}

async function createAliases(search: string = "") {
  const serverMap = {};
  const data = {};

  if (search.length > 0) {
    const siteList: string[] = [];
    const group = "@" + search;
    const sites = await searchSites(search);
    for (const site of sites) {
      const alias = `@${site.name}`;
      data[alias] = {
        ssh: site.ssh,
        path: site.path,
      };
      siteList.push(alias);
    }
    data[group] = siteList;
  } else {
    const serverList = {};
    const servers = await refreshCachedServers();
    const sites = await refreshCachedSites();
    servers.forEach((server: Server) => {
      // Get name as string before first dot.
      const serverAlias = "@" + server.name.split(".")[0];

      serverMap[server.id] = { alias: serverAlias, hostname: server.name };
      serverList[serverAlias] = [];
    });

    sites.forEach((site: Site) => {
      const server = serverMap[site.server_id];
      data[`@${site.domain}`] = createSiteAlias(
        site.site_user,
        server.hostname,
      );
      serverList[server.alias].push(`@${site.domain}`);
    });

    // Merge serverList to end of data
    Object.keys(serverList).forEach((key) => {
      data[key] = serverList[key];
    });
  }

  console.error("Creating aliases...");

  console.log(stringify(data));
}

function createSiteAlias(
  userName: string,
  serverName: string,
  path: string = "files",
) {
  return {
    ssh: `${userName}@${serverName}`,
    path,
  };
}
