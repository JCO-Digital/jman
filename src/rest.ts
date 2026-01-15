import { join } from "path";
import { API_BASE_URL, RequestMethod } from "./constants";
import { config } from "./main";
import { Server, serverSchema } from "./types/server";
import { Site, siteSchema } from "./types/site";
import { SpinupReply } from "./types";

/**
 * Makes a request to the SpinupWP API
 * @param endpoint The endpoint to hit
 * @param method The request method
 * @returns A promise containing the parsed JSON response
 */
export async function makeRequest(
  endpoint: string,
  method: RequestMethod = RequestMethod.GET,
  token: string = config.tokenSpinup,
): Promise<SpinupReply> {
  console.error(`Making a ${method} request to ${endpoint}`);
  const response = await fetch(endpoint, {
    method,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
  });
  return await response.json();
}

export async function getServers() {
  let endpoint: string = join(API_BASE_URL, "servers");
  const servers: Server[] = [];
  try {
    do {
      const payload = await makeRequest(endpoint);
      payload.data
        .map((data: unknown) => serverSchema.parse(data))
        .forEach((server: Server) => {
          servers.push(server);
        });
      if (payload?.pagination?.next) {
        endpoint = payload.pagination.next;
      } else {
        endpoint = "";
      }
    } while (endpoint);
  } catch (e) {
    console.error(e);
  }
  return servers;
}

export async function getSites(): Promise<Site[]> {
  let endpoint: string = join(API_BASE_URL, "sites");
  const sites: Site[] = [];
  try {
    do {
      const payload = await makeRequest(endpoint);
      payload.data
        .map((data: unknown) => siteSchema.parse(data))
        .forEach((site: Site) => {
          sites.push(site);
        });
      if (payload?.pagination?.next) {
        endpoint = payload.pagination.next;
      } else {
        endpoint = "";
      }
    } while (endpoint);
  } catch (e) {
    console.error(e);
  }
  return sites;
}

export async function addMainwpSite(
  url: string,
  admin: string,
  adminpassword: string,
) {
  const endpoint =
    join(config.urlMainwp, "sites/add") +
    `?url=${encodeURIComponent(url)}&admin=${encodeURIComponent(admin)}&adminpassword=${encodeURIComponent(adminpassword)}`;

  const response = await fetch(endpoint, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${config.tokenMainwp}`,
    },
  });
  const data = await response.json();
  if (data.success) {
    console.log(`MainWP site added successfully for ${url}`);
  } else {
    console.error(`Failed to add MainWP site for ${url}: ${data.error}`);
  }
}
