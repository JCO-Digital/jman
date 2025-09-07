import { join } from "path";
import { API_BASE_URL, RequestMethod } from "./constants";
import { config } from "./main";
import { paginationSchema, serverSchema } from "./types";

export function makeRequest(
  endpoint: string,
  method: RequestMethod = RequestMethod.GET,
): Promise<any> {
  return fetch(join(API_BASE_URL, endpoint), {
    method,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${config.token}`,
    },
  }).then((response) => response.json());
}

export function getServers() {
  makeRequest("servers").then((payload) => {
    payload.data
      .map((data) => serverSchema.parse(data))
      .forEach((server) => {
        console.log(server);
      });
    console.log(paginationSchema.parse(payload.pagination));
  });
}

export function getSites() {
  makeRequest("sites").then((payload) => {
    //console.log(payload.data);
    console.log(paginationSchema.parse(payload.pagination));
  });
}
