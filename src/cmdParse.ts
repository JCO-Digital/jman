import {
  getCachedServers,
  getCachedSites,
  refreshCachedServers,
  refreshCachedSites,
} from "./cache";
import { runtimeData } from "./config";
import { cmdSchema, type jCmd } from "./types";
import { Site } from "./types/site";
import { Server } from "./types/server";
import { stringify } from "yaml";
import { join } from "path";

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
      console.log("No command provided.");
      break;
    case "fetch":
      if (data.target.length === 0) {
        console.log("No target provided for fetch command.");
        console.log("Specify: servers, sites or all.");
      }
      if (data.target.includes("all") || data.target.includes("servers")) {
        refreshCachedServers().then((servers) => {
          console.log("Refreshed servers:", servers.length);
        });
      }
      if (data.target.includes("all") || data.target.includes("sites")) {
        refreshCachedSites().then((sites) => {
          console.log("Refreshed sites:", sites.length);
        });
      }
      break;
    case "list":
      console.log("Test");
      break;
    case "alias":
      createAliases();
      break;
    default:
      console.log("Unknown command.");
  }
}

async function createAliases() {
  const servers = await getCachedServers();
  const sites = await getCachedSites();
  const serverMap = {};
  const serverList = {};
  const data = {};

  servers.forEach((server: Server) => {
    // Get name as string before first dot.
    const serverAlias = "@" + server.name.split(".")[0];

    serverMap[server.id] = { alias: serverAlias, hostname: server.name };
    serverList[serverAlias] = [];
  });

  sites.forEach((site: Site) => {
    const server = serverMap[site.server_id];
    data[`@${site.domain}`] = {
      ssh: `${site.site_user}@${server.hostname}`,
      path: join("/sites", site.domain, "files"),
    };
    serverList[server.alias].push(`@${site.domain}`);
  });

  // Merge serverList to end of data
  Object.keys(serverList).forEach((key) => {
    data[key] = serverList[key];
  });

  console.error("Creating aliases...");

  console.log(stringify(data));
}
