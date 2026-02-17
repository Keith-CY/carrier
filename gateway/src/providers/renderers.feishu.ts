import type { GatewayResponse } from "../contracts/commands";

export type FeishuRenderedMessage = {
  msg_type: "text";
  content: {
    text: string;
  };
};

export function renderFeishuResponse(response: GatewayResponse): FeishuRenderedMessage {
  const lines: string[] = [];

  if (response.result === "ok") {
    lines.push(`✅ ${response.message}`);
    if (response.downloadUrl) {
      lines.push(`Download: ${response.downloadUrl}`);
    }
    if (response.handoffId) {
      lines.push(`Handoff: ${response.handoffId} (${response.handoffStatus ?? "pending"})`);
    }
  } else {
    const code = response.errorCode ?? "E_UNKNOWN";
    lines.push(`❌ ${code}: ${response.message}`);
  }

  return {
    msg_type: "text",
    content: {
      text: lines.join("\n"),
    },
  };
}
