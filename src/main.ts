#!/usr/bin/env node

import { readJSONCache, writeJSONCache } from "./cache";
import { readConfigFile } from "./config";
import { getServers, getSites } from "./rest";

export const config = readConfigFile();

function main() {
  getSites();
  //getCachedServers();
  console.log("Hello, World!");
}

function getCachedServers() {
  let servers = readJSONCache("servers");
  if (!servers.length) {
    servers = getServers();
    console.log(servers);
    writeJSONCache("servers", servers);
  }
}

main();
