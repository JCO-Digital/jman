import { WebClient } from "@slack/web-api";
import { config } from "./jman";

export async function sendMessage(message: string) {
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
