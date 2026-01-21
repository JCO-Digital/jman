import { parser, runCmd } from "./cmdParse";
import { readConfigFile } from "./config";
import p from "../package.json";

export const config = readConfigFile();

function main(): void {
  console.warn(`Version: ${p.version}`);
  const cmd = parser(process.argv);
  runCmd(cmd);
}

main();
