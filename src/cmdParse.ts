import { runtimeData } from "./config";
import { cmdSchema, type jCmd } from "./types";
import { setDisallowFileMods } from "./wp-cli";
import { promptSearch } from "./search";
import { hasMainWP } from "./utils";
import {
  addAdmin,
  createAliases,
  fetchData,
  installPlugin,
  listData,
  listInactiveSites,
  mainWPInstall,
  runWPCmd,
  scanVulnerabilities,
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

export async function runCmd(data: jCmd) {
  const currentCmd = commands[data.cmd];
  if (!currentCmd) {
    if (data.cmd !== "") {
      console.error(`Command '${data.cmd}' not found\n`);
    }
    console.warn("Available commands:");
    for (const [key, command] of Object.entries(commands)) {
      if (!command.mainwp || hasMainWP())
        console.warn(`${key}: ${command.description}`);
    }
    return;
  } else {
    if (!currentCmd.mainwp || hasMainWP()) {
      await currentCmd.command(data);
    } else {
      console.error(`Command '${data.cmd}' needs MainWP token.\n`);
    }
  }
}

type commandItem = {
  description: string;
  command: (data: jCmd) => Promise<void>;
  mainwp?: boolean;
};

const commands: Record<string, commandItem> = {
  fetch: {
    description: "Fetch data from SpinupWP.",
    command: fetchData,
  },
  list: {
    description: "List data from SpinupWP. (not fully implemented)",
    command: listData,
  },
  wp: {
    description: "Run a command on wp-cli.",
    command: runWPCmd,
  },
  mainwp: {
    description: "Install MainWP on sites.",
    command: mainWPInstall,
    mainwp: true,
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
  vuln: {
    description: "Scan for vulnerabilities.",
    command: scanVulnerabilities,
  },
};
