import { decode } from "html-entities";
import { versionIsNotBigger } from "./utils";
import { vulnReportSchema } from "./types/vuln";
import type { VulnReport } from "./types/vuln";
import { getSiteList } from "./search";
import { getCachedPluginData, getCachedVulnerabilities } from "./cache";

/**
 * Processes all cached plugins to identify vulnerabilities affecting sites.
 * - Fetches vulnerability data for each plugin.
 * - Matches vulnerable version ranges against installed plugin versions.
 * - Returns a list of reports for vulnerabilities that affect at least one site.
 *
 * @returns An array of VulnReport objects containing affected sites.
 */
export async function processVulnerabilities(): Promise<VulnReport[]> {
  const reports: VulnReport[] = [];

  for (const plugin of await getCachedPluginData()) {
    console.warn(`Processing plugin: ${plugin.name}`);
    const vuln = await getCachedVulnerabilities(plugin.name);

    if (vuln?.data?.vulnerability) {
      for (const vulnerability of vuln.data.vulnerability) {
        const report = vulnReportSchema.parse({
          plugin: vuln.data.name,
          vulnerability,
          sites: [],
        });
        const min = vulnerability.operator.min_version ?? "0";
        const max = vulnerability.operator.max_version ?? "";
        for (const site of plugin.sites) {
          if (
            versionIsNotBigger(site.version, max) &&
            versionIsNotBigger(min, site.version)
          ) {
            report.sites.push(site);
          }
        }
        if (report.sites.length > 0) {
          reports.push(report);
        }
      }
    }
  }
  return reports;
}

/**
 * Formats a vulnerability report into a human-readable string.
 * - Includes plugin name, vulnerability name, and CVSS score.
 * - Lists all affected sites with their plugin versions.
 *
 * @param report - The vulnerability report to format.
 * @returns A formatted string representation of the report.
 */
export async function formatReport(report: VulnReport): Promise<string> {
  const cvss = getCvss(report);
  let formattedReport = `Plugin: ${decode(report.plugin)}\n`;
  formattedReport += `Vulnerability: ${decode(report.vulnerability.name)}\n`;
  if (cvss > 0) {
    formattedReport += `CVS Score: ${cvss}\n`;
  }
  // List sites affected.
  formattedReport += `Affected Sites:\n`;
  for (const site of report.sites) {
    const siteName = await getSiteName(site.site_id);
    formattedReport += `  - ${siteName} (${site.version})\n`;
  }
  return formattedReport;
}

/**
 * Retrieves the site name for a given site ID.
 *
 * @param siteId - The numeric ID of the site.
 * @returns The site name, or an empty string if not found.
 */
export async function getSiteName(siteId: number): Promise<string> {
  for (const site of await getSiteList()) {
    if (site.id === siteId) {
      return site.name;
    }
  }
  return "";
}

/**
 * Extracts the CVSS score from a vulnerability report.
 *
 * @param report - The vulnerability report.
 * @returns The CVSS score as a number, or 0 if not available.
 */
export function getCvss(report: VulnReport): number {
  if (!report.vulnerability.impact?.cvss?.score) {
    return 0;
  }

  // parse string to number.
  return parseFloat(report.vulnerability.impact.cvss.score);
}
