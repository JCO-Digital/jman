import {
  getCachedServers,
  getCachedSites,
  refreshCachedServers,
  refreshCachedSites,
} from "./cache";
import { runtimeData } from "./config";
import { cmdSchema, type jCmd } from "./types";
import { setDisallowFileMods } from "./wp-cli";
import { promptSearch } from "./search";
import {
  addAdmin,
  createAliases,
  installPlugin,
  listInactiveSites,
  mainWPInstall,
  runWPCmd,
  searchTerm,
} from "./commands";

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
    command: () => {
      refreshCachedServers().then((servers) => {
        console.log("Refreshed servers:", servers.length);
      });
      refreshCachedSites().then((sites) => {
        console.log("Refreshed sites:", sites.length);
      });
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
    command: runWPCmd,
  },
  mainwp: {
    description: "Install MainWP on sites.",
    command: mainWPInstall,
  },
  search: {
    description: "Search for a term in sites.",
    command: searchTerm,
  },
  alias: {
    description: "Create alias file for all sites, or a custom collection.",
    command: createAliases,
  },
  inactive: {
    description: "List inactive sites.",
    command: listInactiveSites,
  },
  mods: {
    description: "Set disallow file mods.",
    command: async (data: jCmd) => {
      for (const site of await promptSearch(data.target)) {
        setDisallowFileMods(site.ssh, site.path);
      }
    },
  },
  plugin: {
    description: "Install a plugin.",
    command: installPlugin,
  },
  admin: {
    description: "Create admin user.",
    command: addAdmin,
  },
};
