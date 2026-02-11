import { WebClient } from "@slack/web-api";
import { config } from "./jman";
import { readJSONData, writeJSONData } from "./data";
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

function checkSent(message: string, timeout = 604800000) {
  const crc = crc32(message).toString();
  const sentData: Map<string, number> = new Map(
    Object.entries(readJSONData(sentDataFile, {})),
  );
  const timestamp = sentData.get(crc);
  if (timestamp && Date.now() - timestamp < timeout) {
    return true;
  }
  sentData.set(crc, Date.now());
  writeJSONData(sentDataFile, Object.fromEntries(sentData));
  return false;
}

export function cleanSent(timeout = 604800000) {
  const sentData: Map<string, number> = new Map(
    Object.entries(readJSONData(sentDataFile, {})),
  );
  const now = Date.now();
  for (const [key, value] of sentData) {
    if (now - value > timeout) {
      sentData.delete(key);
    }
  }
  writeJSONData(sentDataFile, Object.fromEntries(sentData));
}
