import { describe, expect, test } from "bun:test";
import type { GatewayResponse } from "../contracts/commands";
import { InMemoryDaemonClient } from "../daemon/client";
import { DownloadTokenStore } from "../downloads/token_store";
import { safeHandleCommand, type GatewayDependencies } from "../index";
import { SessionStore } from "../session/store";
import { renderFeishuResponse } from "./renderers.feishu";

function buildDeps(): GatewayDependencies {
  return {
    daemon: new InMemoryDaemonClient(),
    sessions: new SessionStore(),
    downloads: new DownloadTokenStore(),
  };
}

describe("renderFeishuResponse", () => {
  test("renders success response", () => {
    const response: GatewayResponse = {
      requestId: "req-1",
      result: "ok",
      message: "start completed for openclaw",
    };
    const rendered = renderFeishuResponse(response);
    expect(rendered.msg_type).toBe("text");
    expect(rendered.content.text).toBe("✅ start completed for openclaw");
  });

  test("renders error response with code", () => {
    const response: GatewayResponse = {
      requestId: "req-2",
      result: "error",
      errorCode: "E_SESSION_REQUIRED",
      message: "chat is not paired",
    };
    const rendered = renderFeishuResponse(response);
    expect(rendered.content.text).toBe("❌ E_SESSION_REQUIRED: chat is not paired");
  });

  test("renders download and handoff metadata", () => {
    const response: GatewayResponse = {
      requestId: "req-3",
      result: "ok",
      message: "remote diagnosis consent recorded",
      handoffId: "handoff-99",
      handoffStatus: "declined",
      downloadUrl: "/downloads/dl-99/openclaw.zip",
    };
    const rendered = renderFeishuResponse(response);
    expect(rendered.content.text).toContain("Download: /downloads/dl-99/openclaw.zip");
    expect(rendered.content.text).toContain("Handoff: handoff-99 (declined)");
  });
});

describe("feishu renderer integration path", () => {
  test("pair and agents responses render for feishu", async () => {
    const deps = buildDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("pair-ok");
    }

    const pair = await safeHandleCommand("feishu chat-1 req-pair /pair pair-ok", deps);
    expect(pair.result).toBe("ok");

    const token = pair.sessionToken!;
    const agents = await safeHandleCommand(`feishu chat-1 req-agents ${token} /agents`, deps);
    expect(agents.result).toBe("ok");

    const rendered = renderFeishuResponse(agents);
    expect(rendered.msg_type).toBe("text");
    expect(rendered.content.text).toContain("✅");
    expect(rendered.content.text).toContain("listed");
  });
});
