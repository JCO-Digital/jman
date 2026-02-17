import { join } from "path";
import { addMainwpSite } from "./rest";
import { promptSearch, searchSites } from "./search";
import { jCmd } from "./types";
import {
  addPlugin,
  addUser,
  isActiveMainwp,
  resetUserPassword,
  runWP,
} from "./wp-cli";
import { stringify } from "yaml";
import { REPO_PATH } from "./constants";
import {
  getCachedPlugins,
  getCachedServers,
  getCachedSites,
  getCachedVulnerabilities,
  refreshCachedServers,
  refreshCachedSites,
} from "./cache";
import { config } from "./jman";
import { runtimeData } from "./config";
import { getLatestVersion, versionIsNotBigger } from "./utils";
import { downloadReleaseByTag } from "./fileHelpers";
import { chmodSync, existsSync, renameSync, unlinkSync } from "fs";

interface SiteAlias {
  ssh: string;
  path: string;
}

interface ServerInfo {
  alias: string;
  hostname: string;
}

type AliasData = SiteAlias | string[];

interface AliasRegistry {
  [key: string]: AliasData;
}

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

/**
 * Creates WP-CLI aliases for sites and servers.
 * - If a search target is provided, creates aliases for matching sites and a group alias.
 * - If no search target is provided, creates aliases for all sites and server groups.
 * - Outputs the alias registry in YAML format.
 *
 * @param cmdData - The command data containing the search target.
 */
export async function createAliases(cmdData: jCmd): Promise<void> {
  const search = cmdData.target;
  const aliasRegistry: AliasRegistry = {};

  if (search.length > 0) {
    // Handle specific search query
    await createSearchAliases(search, aliasRegistry);
  } else {
    // Handle all sites and servers
    await createAllAliases(aliasRegistry);
  }

  console.warn("Creating aliases...");
  console.log(stringify(aliasRegistry));
}

/**
 * Creates aliases for sites matching a specific search query.
 * - Creates individual site aliases (@sitename).
 * - Creates a group alias (@searchterm) containing all matching sites.
 *
 * @param search - The search string to match sites.
 * @param registry - The alias registry to populate.
 */
async function createSearchAliases(
  search: string,
  registry: AliasRegistry,
): Promise<void> {
  const siteAliases: string[] = [];
  const groupAlias = `@${search}`;
  const sites = await searchSites(search);

  for (const site of sites) {
    const alias = `@${site.name}`;
    registry[alias] = createSiteAlias(site.ssh, site.name, site.path);
    siteAliases.push(alias);
  }

  registry[groupAlias] = siteAliases;
}

/**
 * Creates aliases for all sites and servers.
 * - Creates individual site aliases (@domain).
 * - Creates server group aliases (@servername) containing all sites on that server.
 *
 * @param registry - The alias registry to populate.
 */
async function createAllAliases(registry: AliasRegistry): Promise<void> {
  const serverInfoMap = new Map<number, ServerInfo>();
  const serverAliasLists = new Map<string, string[]>();

  // Fetch and process servers
  const servers = await refreshCachedServers();
  for (const server of servers) {
    const serverAlias = `@${server.name.split(".")[0]}`;
    serverInfoMap.set(server.id, {
      alias: serverAlias,
      hostname: server.name,
    });
    serverAliasLists.set(serverAlias, []);
  }

  // Fetch and process sites
  const sites = await refreshCachedSites();
  for (const site of sites) {
    const serverInfo = serverInfoMap.get(site.server_id);

    if (!serverInfo) {
      console.warn(
        `Server not found for site ${site.domain} (server_id: ${site.server_id})`,
      );
      continue;
    }

    const siteAlias = `@${site.domain}`;
    registry[siteAlias] = createSiteAlias(site.site_user, serverInfo.hostname);

    const serverList = serverAliasLists.get(serverInfo.alias);
    if (serverList) {
      serverList.push(siteAlias);
    }
  }

  // Add server group aliases to registry
  for (const [serverAlias, siteList] of serverAliasLists.entries()) {
    registry[serverAlias] = siteList;
  }
}

/**
 * Creates a site alias object with SSH connection details.
 *
 * @param userName - The SSH username for the site.
 * @param serverName - The hostname of the server.
 * @param path - The path to the site files (defaults to "files").
 * @returns A SiteAlias object with ssh and path properties.
 */
function createSiteAlias(
  userName: string,
  serverName: string,
  path: string = "files",
): SiteAlias {
  return {
    ssh: `${userName}@${serverName}`,
    path,
  };
}

/**
 * Fetches and refreshes cached server and site data.
 * Calls refreshCachedServers and refreshCachedSites, then logs the number of servers and sites refreshed.
 */
export async function fetchData(data: jCmd) {
  const target = data.target ? data.target : "basic";

  if (target === "basic" || target === "servers" || target === "all") {
    const servers = await refreshCachedServers();
    console.log("Fetched servers:", servers.length);
  }
  if (target === "basic" || target === "sites" || target === "all") {
    const sites = await refreshCachedSites();
    console.log("Fetched sites:", sites.length);
  }
  if (target === "plugins" || target === "all") {
    const plugins = await getCachedPlugins(true);
    console.log("Fetched plugins:", plugins.length);
  }
  if (target === "vuln" || target === "all") {
    const pluginList: string[] = [];
    for (const plugin of await getCachedPlugins()) {
      if (!pluginList.includes(plugin.name)) {
        pluginList.push(plugin.name);
      }
    }
    for (const plugin of pluginList) {
      const vulns = await getCachedVulnerabilities(plugin, true);
      console.log(
        `Fetched vulnerabilities for ${plugin}:`,
        vulns?.data?.vulnerability?.length ?? 0,
      );
    }
  }
}

/**
 * Lists cached servers and/or sites based on the target parameter.
 * - Target "servers": Lists all cached server names.
 * - Target "sites": Lists all cached site domains.
 * - Target "all": Lists both servers and sites.
 *
 * @param data - The command data containing the target ("servers", "sites", or "all").
 */
export async function listData(data: jCmd) {
  if (data.target === "") {
    console.error("No target provided for list command.");
    console.error("Specify: servers, sites or all.");
  }
  if (data.target === "all" || data.target === "servers") {
    getCachedServers().then((servers) => {
      console.warn("\nCached servers:", servers.length);
      for (const server of servers) {
        console.log(server.name);
      }
    });
  }
  if (data.target === "all" || data.target === "sites") {
    getCachedSites().then((sites) => {
      console.warn("\nCached sites:", sites.length);
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
      console.warn(
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

/**
 * Executes a WP-CLI command on all sites matching the search criteria.
 * - Searches for sites using promptSearch(data.target).
 * - Runs the command specified in data.args on each site.
 * - Logs the output or any errors encountered.
 *
 * @param data - The command data containing search parameters and the WP-CLI command arguments.
 */
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

/**
 * Searches for sites matching the provided search term and displays results.
 * - Uses searchSites to find matching sites.
 * - Displays site name and server name for each result.
 *
 * @param data - The command data containing the search term in data.target.
 */
export async function searchTerm(data: jCmd) {
  searchSites(data.target).then((sites) => {
    console.log("Search results:");
    sites.forEach((site) => {
      console.log(`${site.name} (${site.serverName})`);
    });
  });
}

/**
 * Installs a WordPress plugin on all sites matching the search criteria.
 * - Requires at least one argument: the plugin name or URL.
 * - Supports SatisPress repository URLs for private plugins.
 * - For each site found by promptSearch(data.target), installs the specified plugin.
 *
 * @param data - The command data containing search parameters and plugin name/URL.
 */
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

/**
 * Processes a plugin identifier and returns the appropriate plugin name or path.
 * - If the plugin is a SatisPress repository URL, extracts and constructs the local file path.
 * - Otherwise, returns the plugin name as-is.
 *
 * @param plugin - The plugin name or SatisPress URL.
 * @returns The processed plugin name or file path.
 */
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

export async function updateJman() {
  const currentVersion = runtimeData.version;
  const latestTag = await getLatestVersion();
  const latestVersion = latestTag.slice(1);
  if (versionIsNotBigger(latestVersion, currentVersion)) {
    console.log("jman is up to date.");
    return;
  }

  console.log(`Updating jman from ${currentVersion} to ${latestVersion}...`);

  // Check if scriptPath ends with ".ts"
  if (
    runtimeData.scriptPath.endsWith(".ts") &&
    !runtimeData.execPath.endsWith("/jman")
  ) {
    console.error("Can only update bundled version!");
    return;
  }
  const tempFile = `${runtimeData.execPath}.part`;
  const renamedFile = `${runtimeData.execPath}.bak`;
  downloadReleaseByTag(latestTag, tempFile)
    .then(() => {
      renameSync(runtimeData.execPath, renamedFile);
      renameSync(tempFile, runtimeData.execPath);
      chmodSync(runtimeData.execPath, 0o755);
      unlinkSync(renamedFile);
      console.log(
        `Updated new version ${latestVersion} to ${runtimeData.execPath}`,
      );
    })
    .catch((error) => {
      console.error(`Failed to update jman: ${error}`);
      if (existsSync(tempFile)) {
        unlinkSync(tempFile);
      }
    });
}
