#!/usr/bin/env node

import { getCachedSites } from "./cache";
import { readConfigFile } from "./config";

export const config = readConfigFile();

async function main() {
  console.log(getCachedSites());
  console.log("Hello, World!");
}

main();
