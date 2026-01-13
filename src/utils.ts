import { config } from "./main";

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
