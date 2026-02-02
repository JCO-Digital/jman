import { parser, runCmd } from "./cmdParse";
import { readConfigFile, runtimeData } from "./config";

export const config = readConfigFile();

function main(): void {
  console.warn(`Version: ${runtimeData.version}`);
  const cmd = parser(process.argv);
  runCmd(cmd);
}

main();
