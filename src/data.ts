import { dirname, join } from "path";
import { runtimeData } from "./config";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "fs";

export function readJSONData(filename: string, defaultValue: object = {}) {
  const filePath = getJSONFilename(filename);

  // If file does not exist, return default value
  if (!existsSync(filePath)) {
    return defaultValue;
  }

  try {
    return JSON.parse(readFileSync(filePath, "utf8"));
  } catch (error) {
    console.error(error);
    return defaultValue;
  }
}

export function writeJSONData(filename: string, data: object) {
  const filePath = getJSONFilename(filename);

  // If folder does not exist, create it
  const folderPath = dirname(filePath);
  if (!existsSync(folderPath)) {
    mkdirSync(folderPath, { recursive: true });
  }

  try {
    writeFileSync(filePath, JSON.stringify(data, null, 2), "utf8");
  } catch (error) {
    console.error(`Failed to write data file ${filename}:`, error);
  }
}

function getJSONFilename(filename: string): string {
  return join(runtimeData.dataDir, `${filename}.json`);
}
