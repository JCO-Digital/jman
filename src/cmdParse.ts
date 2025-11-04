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
import { addPlugin, addUser, isActiveMainwp, runWP } from "./wp-cli";
import { promptSearch, searchSites } from "./search";
import { addMainwpSite } from "./rest";

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
  // Get Target from next arg.
  if (args.length > 0) {
    cmdData.target = args.shift() ?? "";
  }
  // Append remaining args.
  if (args.length > 0) {
    cmdData.args = args;
  }

  return cmdData;
}

export function runCmd(data: jCmd) {
  const currentCmd = commands[data.cmd];
  if (currentCmd) {
    currentCmd.command(data);
  } else {
    if (data.cmd !== "") {
      console.error(`Command '${data.cmd}' not found\n`);
    }
    console.error("Available commands:");
    for (const cmd in commands) {
      console.error(`${cmd}: ${commands[cmd].description}`);
    }
  }
}

const commands = {
  fetch: {
    description: "Fetch data from SpinupWP.",
    command: (data: jCmd) => {
      if (data.target === "") {
        console.error("No target provided for fetch command.");
        console.error("Specify: servers, sites or all.");
      }
      if (data.target === "all" || data.target === "servers") {
        refreshCachedServers().then((servers) => {
          console.error("Refreshed servers:", servers.length);
        });
      }
      if (data.target === "all" || data.target === "sites") {
        refreshCachedSites().then((sites) => {
          console.error("Refreshed sites:", sites.length);
        });
      }
    },
  },
  list: {
    description: "List data from SpinupWP. (not fully implemented)",
    command: (data: jCmd) => {
      if (data.target === "") {
        console.error("No target provided for list command.");
        console.error("Specify: servers, sites or all.");
      }
      if (data.target === "all" || data.target === "servers") {
        getCachedServers().then((servers) => {
          console.error("Cached servers:", servers.length);
        });
      }
      if (data.target === "all" || data.target === "sites") {
        getCachedSites().then((sites) => {
          console.error("Cached sites:", sites.length);
        });
      }
    },
  },
  wp: {
    description: "Run a command on wp-cli.",
    command: (data: jCmd) => {
      try {
        runWPCmd(data.target, data.args.join(" "));
      } catch (error) {
        console.error("Error running WP command:", error);
      }
    },
  },
  mainwp: {
    description: "Install MainWP on sites.",
    command: (data: jCmd) => {
      mainWPInstall(data.target);
    },
  },
  search: {
    description: "Search for a term in sites.",
    command: (data: jCmd) => {
      searchSites(data.target).then((sites) => {
        console.log("Search results:");
        sites.forEach((site) => {
          console.log(`${site.name} (${site.serverName})`);
        });
      });
    },
  },
  alias: {
    description: "Create alias file for all sites, or a custom collection.",
    command: (data: jCmd) => {
      createAliases(data.target);
    },
  },
  inactive: {
    description: "List inactive sites.",
    command: (data: jCmd) => {
      listInactiveSites(data.target);
    },
  },
};

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

async function mainWPInstall(search: string) {
  const searchResults = await promptSearch(search);
  for (const site of searchResults) {
    const active = await isActiveMainwp(site.ssh, site.path);
    if (active) {
      console.log(`MainWP is already active for ${site.name}`);
      continue;
    }
    console.log(`Installing MainWP for ${site.name}`);
    try {
      console.log("Installing MainWP user");
      const password = await addUser(
        site.ssh,
        site.path,
        "mainwp",
        "mainwp@jco.fi",
        "administrator",
      );

      console.log("Installing MainWP Child Plugin");
      if (!(await addPlugin(site.ssh, site.path, "mainwp-child"))) {
        console.log("MainWP Child Plugin failed to install.");
        continue;
      }

      console.log("Adding site to MainWP");
      await addMainwpSite(`https://${site.name}`, "mainwp", password);
    } catch (error) {
      if (error.toString().includes("username is already registered")) {
        console.log(`MainWP user already exists for ${site.name}`);
      } else {
        console.error(`Error installing MainWP for ${site.name}`);
      }
      continue;
    }
  }
}
async function listInactiveSites(search: string) {
  const inactive: string[] = [];
  for (const site of await promptSearch(search)) {
    console.log(`\nChecking ${site.name} (${site.serverName})`);
    if (await isActiveMainwp(site.ssh, site.path)) {
      console.log(`Already active`);
    } else {
      console.log("Not active, or connection error.");
      inactive.push(`${site.name} (${site.serverName})`);
    }
  }

  if (inactive.length > 0) {
    console.log(`\nInactive sites:`);
    for (const site of inactive) {
      console.log(site);
    }
  }
}
