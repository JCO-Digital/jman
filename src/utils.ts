import { join } from "path";
import { config } from "./jman";
import { writeFile } from "fs";
import { getFile } from "./fileHelpers";

export function hasMainWP(): boolean {
  return config.tokenMainwp.length > 0;
}

export function getErrorMessage(error: unknown) {
  if (typeof error === "string") {
    return error;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return "Unknown error";
}

export function versionIsNotBigger(plugin: string, vuln: string): boolean {
  const pluginParts = plugin.split(".");
  const vulnParts = vuln.split(".");

  for (let i = 0; i < Math.max(pluginParts.length, vulnParts.length); i++) {
    const pluginPart = parseInt(pluginParts[i] || "0");
    const vulnPart = parseInt(vulnParts[i] || "0");

    if (pluginPart > vulnPart) return false;
    if (pluginPart < vulnPart) return true;
  }

  return true;
}

export function getLatestVersion(): Promise<string> {
  return new Promise((resolve, reject) => {
    const url = "https://api.github.com/repos/JCO-Digital/jman/releases/latest";
    const options = {
      headers: {
        "User-Agent": "jman",
      },
    };

    fetch(url, options)
      .then((response) => response.json())
      .then((data) => resolve(data.tag_name))
      .catch((error) => reject(error));
  });
}
