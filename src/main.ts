#!/usr/bin/env node

import { parser, runCmd } from "./cmdParse";
import { readConfigFile } from "./config";
import p from "../package.json";

export const config = readConfigFile();

async function main() {
  console.error(`Version: ${p.version}`);
  const cmd = parser(process.argv);
  runCmd(cmd);
}

main();
