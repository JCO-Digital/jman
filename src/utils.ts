import { config } from "./main";

export function hasMainWP(): boolean {
  return config.tokenMainwp.length > 0;
}
