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
): Promise<SpinupReply> {
  console.debug(`Making a ${method} request to ${endpoint}`);
  const response = await fetch(endpoint, {
    method,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${config.token}`,
    },
  });
  return await response.json();
}

export function getServers() {
  makeRequest("servers").then((payload) => {
    payload.data
      .map((data: unknown) => serverSchema.parse(data))
      .forEach((server: Server) => {
        console.log(server);
      });
    console.log(payload.pagination);
  });
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
    } while (endpoint && false);
  } catch (e) {
    console.error(e);
  }
  return sites;
}
