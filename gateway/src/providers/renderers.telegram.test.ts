import { describe, expect, test } from "bun:test";
import type { GatewayResponse } from "../contracts/commands";
import { InMemoryDaemonClient } from "../daemon/client";
import { DownloadTokenStore } from "../downloads/token_store";
import { safeHandleCommand, type GatewayDependencies } from "../index";
import { SessionStore } from "../session/store";
import { renderTelegramResponse } from "./renderers";

function buildDeps(): GatewayDependencies {
  return {
    daemon: new InMemoryDaemonClient(),
    sessions: new SessionStore(),
    downloads: new DownloadTokenStore(),
  };
}

describe("renderTelegramResponse", () => {
  test("renders success response with session token", () => {
    const response: GatewayResponse = {
      requestId: "req-1",
      result: "ok",
      message: "paired telegram:100",
      sessionToken: "session-abc",
    };

    const rendered = renderTelegramResponse(response);
    expect(rendered.text).toContain("✅ paired telegram:100");
    expect(rendered.text).toContain("Session token: session-abc");
  });

  test("renders error response with code", () => {
    const response: GatewayResponse = {
      requestId: "req-2",
      result: "error",
      errorCode: "E_SESSION_REQUIRED",
      message: "chat is not paired",
    };

    const rendered = renderTelegramResponse(response);
    expect(rendered.text).toBe("❌ E_SESSION_REQUIRED: chat is not paired");
  });

  test("renders download and handoff details", () => {
    const response: GatewayResponse = {
      requestId: "req-3",
      result: "ok",
      message: "remote diagnosis consent recorded for openclaw",
      handoffId: "handoff-1",
      handoffStatus: "pending",
      downloadUrl: "/downloads/dl-1/openclaw.zip",
    };

    const rendered = renderTelegramResponse(response);
    expect(rendered.text).toContain("Handoff: handoff-1 (pending)");
    expect(rendered.text).toContain("Download: /downloads/dl-1/openclaw.zip");
    expect(rendered.disableWebPagePreview).toBe(true);
  });
});

describe("telegram renderer integration path", () => {
  test("pair and agents responses render into telegram-friendly text", async () => {
    const deps = buildDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("pair-ok");
    }

    const pair = await safeHandleCommand("telegram 100 req-pair /pair pair-ok", deps);
    expect(pair.result).toBe("ok");
    const pairRendered = renderTelegramResponse(pair);
    expect(pairRendered.text).toContain("✅ paired telegram:100");

    const token = pair.sessionToken!;
    const agents = await safeHandleCommand(`telegram 100 req-agents ${token} /agents`, deps);
    expect(agents.result).toBe("ok");
    const agentsRendered = renderTelegramResponse(agents);
    expect(agentsRendered.text).toContain("✅");
    expect(agentsRendered.text).toContain("listed");
  });
});
