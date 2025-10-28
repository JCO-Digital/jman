#!/usr/bin/env node

import { parser, runCmd } from "./cmdParse";
import { readConfigFile } from "./config";

export const config = readConfigFile();

async function main() {
  const cmd = parser(process.argv);
  console.error(cmd);
  runCmd(cmd);
}

main();
