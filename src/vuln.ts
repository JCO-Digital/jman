import { decode } from "html-entities";
import { versionIsNotBigger } from "./utils";
import { vulnPluginSchema, vulnReportSchema } from "./types/vuln";
import type { VulnPlugin, VulnReport } from "./types/vuln";
import { getSiteList } from "./search";
import { getCachedPluginData, getCachedVulnerabilities } from "./cache";
import { jCmd } from "./types";
import { config } from "./jman";
import { sendMessage } from "./slack";

/**
 * Scans for plugin vulnerabilities across all sites.
 * - Target "cvss": Filters vulnerabilities by CVSS score threshold.
 * - Target "slack": Sends vulnerability reports to Slack (new or high-severity only).
 * - Processes all cached plugins and checks for known vulnerabilities.
 * - Tracks sent Slack messages to avoid duplicates.
 *
 * @param data - The command data containing the target ("cvss" or "slack") and optional CVSS threshold.
 */
export async function scanVulnerabilities(data: jCmd) {
  if (data.target === "sites") {
    const sites = await buildSiteList();

    let siteCount = 0;
    for (const [site_id, site] of sites.entries()) {
      let cvss = 0;
      let vulns = 0;

      for (const plugin of site.values()) {
        vulns += plugin.vulnerability?.length || 0;
        if (plugin.cvss && plugin.cvss > cvss) {
          cvss = plugin.cvss;
        }
      }
      if (cvss > config.cvssThreshold || vulns > config.vulnThreshold) {
        const siteName = await getSiteName(site_id);
        if (config.ignoreSites.includes(siteName)) {
          continue;
        }
        siteCount++;
        const message = formatSiteReport(
          `${siteName} (${vulns} Vulnerabilities)`,
          site,
        );

        console.log(message);

        if (data.args.includes("slack")) {
          await sendMessage(message);
        }
      }
    }
    console.warn(`${siteCount} sites match criteria`);
  } else {
    for (const report of await processVulnerabilities()) {
      const cvss = getCvss(report);
      if (data.target === "cvss") {
        let cvssThreshold = config.cvssThreshold;
        if (data.args[0]) {
          cvssThreshold = parseFloat(data.args[0]);
        }
        if (cvss < cvssThreshold) {
          continue;
        }
      }

      const message = await formatReport(report);
      console.log(message);
      if (data.target === "slack") {
        await sendMessage(message, cvss >= config.cvssThreshold);
      }
    }
  }
}

async function buildSiteList(): Promise<Map<number, Map<string, VulnPlugin>>> {
  const sites = new Map<number, Map<string, VulnPlugin>>();

  for (const report of await processVulnerabilities()) {
    const cvss = getCvss(report);
    for (const site of report.sites) {
      let currentSite = sites.get(site.site_id);

      if (!currentSite) {
        currentSite = new Map<string, VulnPlugin>();
        sites.set(site.site_id, currentSite);
      }

      let currentPlugin = currentSite.get(report.plugin);

      if (!currentPlugin) {
        currentPlugin = vulnPluginSchema.parse({
          version: site.version,
          cvss: cvss,
          vulnerability: [],
        });
        currentSite.set(report.plugin, currentPlugin);
      }

      if (currentPlugin.cvss === null || cvss > currentPlugin.cvss) {
        currentPlugin.cvss = cvss;
      }

      currentPlugin.vulnerability?.push(report.vulnerability);
    }
  }

  return sites;
}

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
  const info = {
    name: "",
    description: report.vulnerability.description,
    date: "",
    link: "",
  };

  let formattedReport = `Plugin: ${decode(report.plugin)}\n`;
  for (const source of report.vulnerability.source) {
    if (!info.name && !source.name.startsWith("CVE")) {
      info.name = source.name;
    }
    if (!info.description && source.description) {
      info.description = source.description;
    }
    if (!info.date && source.date) {
      info.date = source.date;
    }
    if (!info.link && source.link) {
      info.link = source.link;
    }
  }
  if (!info.name) {
    info.name = report.vulnerability.name;
  }
  formattedReport += `Vulnerability: ${decode(info.name)}\n`;
  if (info.date) {
    formattedReport += `Date: ${info.date}\n`;
  }
  if (cvss > 0) {
    formattedReport += `CVS Score: ${cvss}\n`;
  }
  if (info.description) {
    formattedReport += `Description: ${decode(info.description)}\n`;
  }
  // List sites affected.
  formattedReport += `\nAffected Sites:\n`;
  for (const site of report.sites) {
    const siteName = await getSiteName(site.site_id);
    formattedReport += `  - ${siteName} (${site.version})\n`;
  }
  return formattedReport;
}

function formatSiteReport(site: string, plugins: Map<string, VulnPlugin>) {
  let formattedReport = `${decode(site)}\n`;
  for (const [plugin, info] of plugins.entries()) {
    formattedReport += `  ${decode(plugin)} - ${info.version}\n`;
    formattedReport += `    Vulnerabilities: ${info.vulnerability?.length}\n`;
    if (info.cvss) {
      formattedReport += `    Highest CVSS: ${info.cvss}\n`;
    }
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
  if (
    report.vulnerability.impact &&
    report.vulnerability.impact.cvss &&
    report.vulnerability.impact.cvss.score
  ) {
    return parseFloat(report.vulnerability.impact.cvss.score);
  }

  return 0;
}
