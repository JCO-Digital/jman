import xdg from "@folder/xdg";
import { join } from "path";
import { readFileSync, existsSync, mkdirSync } from "fs";
import { parse } from "smol-toml";
import { configSchema, jConfig, runtimeSchema } from "./types";
import { PACKAGE_NAME } from "./constants";

export const runtimeData = runtimeSchema.parse({
  configDir: join(xdg().config, PACKAGE_NAME),
  cacheDir: join(xdg().cache, PACKAGE_NAME),
  dataDir: join(xdg().data, PACKAGE_NAME),
});

export function getConfigFilePath(): string {
  [runtimeData.configDir, runtimeData.cacheDir, runtimeData.dataDir].forEach(
    (path) => {
      if (!existsSync(path)) {
        console.debug(`Creating directory at ${path}`);
        mkdirSync(path, { recursive: true });
      }
    },
  );
  return join(runtimeData.configDir, "config.toml");
}

export function readConfigFile(): jConfig {
  const configPath = getConfigFilePath();

  if (!existsSync(configPath)) {
    console.error(`Config file not found at ${configPath}`);
    process.exit(1);
  }

  const configRaw = readFileSync(configPath, "utf8");
  const config = configSchema.parse(parse(configRaw));

  return config;
}
