import { join } from "path";
import { addMainwpSite } from "./rest";
import { promptSearch, searchSites } from "./search";
import { jCmd } from "./types";
import {
  addPlugin,
  addUser,
  getPlugins,
  isActiveMainwp,
  resetUserPassword,
  runWP,
} from "./wp-cli";
import { stringify } from "yaml";
import { REPO_PATH } from "./constants";
import {
  getCachedPluginData,
  getCachedPlugins,
  getCachedServers,
  getCachedSites,
  refreshCachedServers,
  refreshCachedSites,
} from "./cache";
import { Server } from "./types/server";
import { Site } from "./types/site";
import { config } from "./main";
import { da } from "zod/v4/locales";

/**
 * Adds an administrator user to all sites matching the search criteria.
 * - Requires at least two arguments: username and email.
 * - For each site found by promptSearch(data.target):
 *   - Calls addUser to add the user as an administrator.
 *   - Logs the result of the operation.
 *
 * @param data - The command data containing search parameters and arguments.
 */
export async function addAdmin(data: jCmd) {
  if (data.args.length < 2) {
    console.error("Please provide a username and email.");
    return;
  }
  for (const site of await promptSearch(data.target)) {
    console.log(
      await addUser(
        site.ssh,
        site.path,
        data.args[0],
        data.args[1],
        "administrator",
      ),
    );
  }
}

export async function createAliases(cmdData: jCmd) {
  const search = cmdData.target;
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

/**
 * Fetches and refreshes cached server and site data.
 * Calls refreshCachedServers and refreshCachedSites, then logs the number of servers and sites refreshed.
 */
export async function fetchData() {
  const servers = await refreshCachedServers();
  console.log("Fetched servers:", servers.length);
  const sites = await refreshCachedSites();
  console.log("Fetched sites:", sites.length);
}

export async function listData(data: jCmd) {
  if (data.target === "") {
    console.error("No target provided for list command.");
    console.error("Specify: servers, sites or all.");
  }
  if (data.target === "all" || data.target === "servers") {
    getCachedServers().then((servers) => {
      console.error("\nCached servers:", servers.length);
      for (const server of servers) {
        console.log(server.name);
      }
    });
  }
  if (data.target === "all" || data.target === "sites") {
    getCachedSites().then((sites) => {
      console.error("\nCached sites:", sites.length);
      for (const site of sites) {
        console.log(site.domain);
      }
    });
  }
}

/**
 * Lists inactive sites based on the provided search string.
 * For each site matching the search string in data.target, checks if MainWP is active.
 * If MainWP is not active or there is a connection error, adds the site to the inactive list.
 * At the end, prints all inactive sites found.
 *
 * @param data - The command data containing search parameters..
 */
export async function listInactiveSites(data: jCmd) {
  const inactive: string[] = [];
  for (const site of await promptSearch(data.target)) {
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

/**
 * Installs the MainWP user and MainWP Child plugin on all sites matching the search criteria.
 * - For each site found by promptSearch(data.target):
 *   - Checks if MainWP is already active; skips if so.
 *   - Attempts to add a "mainwp" administrator user; if the user exists, resets its password.
 *   - Installs the "mainwp-child" plugin.
 *   - Adds the site to MainWP using addMainwpSite.
 *   - Logs progress and errors for each step.
 *
 * @param data - The command data containing search parameters and arguments.
 */
export async function mainWPInstall(data: jCmd) {
  if (!config.tokenMainwp) {
    console.error("MainWP token not found");
    return;
  }

  const searchResults = await promptSearch(data.target);
  for (const site of searchResults) {
    const active = await isActiveMainwp(site.ssh, site.path);
    if (active) {
      console.log(`MainWP is already active for ${site.name}`);
      continue;
    }
    console.log(`Installing MainWP for ${site.name}`);
    let password = "";
    try {
      console.log("Installing MainWP user");
      password = await addUser(
        site.ssh,
        site.path,
        "mainwp",
        "mainwp@jco.fi",
        "administrator",
      );
    } catch (_) {
      console.error(
        `MainWP user already exists for ${site.name}, resetting password.`,
      );
      try {
        password = await resetUserPassword(site.ssh, site.path, "mainwp");
      } catch (_) {
        console.error(`Failed to reset password for ${site.name}`);
        continue;
      }
    }

    try {
      console.log("Installing MainWP Child Plugin");
      if (!(await addPlugin(site.ssh, site.path, "mainwp-child"))) {
        console.log("MainWP Child Plugin failed to install.");
        continue;
      }

      console.log("Adding site to MainWP");
      await addMainwpSite(`https://${site.name}`, "mainwp", password);
    } catch (_) {
      console.error(`Error installing MainWP for ${site.name}`);
      continue;
    }
  }
}

export async function runWPCmd(data: jCmd) {
  const searchResults = await promptSearch(data.target);
  const command = data.args.join(" ");
  for (const result of searchResults) {
    console.log(
      `Running command '${command}' on ${result.name} (${result.serverName})`,
    );
    try {
      const ret = await runWP(result.ssh, result.path, command);
      console.log(ret.output);
    } catch (error) {
      console.error(
        `Error running command '${command}' on ${result.name}:`,
        error,
      );
    }
  }
}

export async function searchTerm(data: jCmd) {
  searchSites(data.target).then((sites) => {
    console.log("Search results:");
    sites.forEach((site) => {
      console.log(`${site.name} (${site.serverName})`);
    });
  });
}

export async function installPlugin(data: jCmd) {
  if (data.args.length < 1) {
    console.error("Usage: jman plugin <search> <plugin>");
    return;
  }
  const plugin = getPluginName(data.args[0]);
  for (const site of await promptSearch(data.target)) {
    console.log(`\nInstalling ${plugin} on ${site.name} (${site.serverName})`);
    if (await addPlugin(site.ssh, site.path, plugin, false)) {
      console.log("Plugin installed successfully.");
    }
  }
}

function getPluginName(plugin: string): string {
  const repo = plugin.match(
    /(https:\/\/repo\.jco\.fi)\/satispress\/([^/]+)\/(\d+\.\d+\.\d+)/,
  );
  if (repo) {
    const fileName = join(REPO_PATH, repo[2], repo[2] + "-" + repo[3] + ".zip");
    return repo[1] + fileName;
  }

  return plugin;
}

export async function scanVulnerabilities(data: jCmd) {
  if (data.target) {
    console.error("Site specific check not yet implemented.");
    return;
  }

  for (const plugin of await getCachedPluginData()) {
    console.error(
      `Plugin: ${plugin.name} installed on ${plugin.sites.length} sites.`,
    );
  }
}
