import { WebClient } from "@slack/web-api";
import { config } from "./jman";
import { readMapData, writeMapData } from "./data";
import { crc32 } from "crc";

const sentDataFile = "sentSlackMessages";

export async function sendMessage(message: string, force = false) {
  if (!config.slackToken) {
    console.error("Slack token not configured");
    return;
  }

  if (!force && checkSent(message)) {
    console.warn("Slack message already sent recently");
    return;
  }

  const web = new WebClient(config.slackToken);
  try {
    await web.chat.postMessage({
      channel: config.slackChannel,
      text: message,
    });
    console.warn("Slack message sent successfully");
  } catch (error) {
    console.error("Error sending Slack message:", error);
  }
}

function checkSent(message: string, timeout = "1w"): boolean {
  const crc = crc32(message).toString();
  const sentData: Map<string, number> = readMapData(sentDataFile);
  const timestamp = sentData.get(crc);
  if (timestamp && Date.now() - timestamp < parseTimeout(timeout)) {
    return true;
  }
  sentData.set(crc, Date.now());
  writeMapData(sentDataFile, sentData);
  return false;
}

export function cleanSent(timeout = "1w"): void {
  const sentData: Map<string, number> = readMapData(sentDataFile);
  const now = Date.now();
  for (const [key, value] of sentData) {
    if (now - value > parseTimeout(timeout)) {
      sentData.delete(key);
    }
  }
  writeMapData(sentDataFile, sentData);
}

function parseTimeout(timeout: string): number {
  const match = timeout.match(/^(\d+)([smhdwMy]?)$/);
  if (!match) {
    throw new Error(`Invalid timeout format: ${timeout}`);
  }
  const [, value, unit] = match;
  const numValue = parseInt(value);
  let unitFactor = 0;

  switch (unit) {
    case "":
    case "s":
      unitFactor = 1;
      break;
    case "m":
      unitFactor = 60;
      break;
    case "h":
      unitFactor = 60 * 60;
      break;
    case "d":
      unitFactor = 24 * 60 * 60;
      break;
    case "w":
      unitFactor = 7 * 24 * 60 * 60;
      break;
    case "M":
      unitFactor = 30 * 24 * 60 * 60;
      break;
    case "y":
      unitFactor = 365 * 24 * 60 * 60;
      break;
    default:
      throw new Error(`Invalid timeout unit: ${unit}`);
  }

  return numValue * unitFactor * 1000;
}
