import { config } from "./jman";

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
