import type { GatewayResponse } from "../contracts/commands";

export type TelegramRenderedMessage = {
  text: string;
  disableWebPagePreview?: boolean;
};

export function renderTelegramResponse(response: GatewayResponse): TelegramRenderedMessage {
  const lines: string[] = [];

  if (response.result === "ok") {
    lines.push(`✅ ${response.message}`);
    if (response.sessionToken) {
      lines.push(`Session token: ${response.sessionToken}`);
    }
    if (response.handoffId) {
      lines.push(`Handoff: ${response.handoffId} (${response.handoffStatus ?? "pending"})`);
    }
    if (response.downloadUrl) {
      lines.push(`Download: ${response.downloadUrl}`);
    }
  } else {
    const code = response.errorCode ?? "E_UNKNOWN";
    lines.push(`❌ ${code}: ${response.message}`);
  }

  return {
    text: lines.join("\n"),
    disableWebPagePreview: Boolean(response.downloadUrl),
  };
}
