import { describe, expect, test } from "bun:test";
import type { GatewayResponse } from "../contracts/commands";
import { InMemoryDaemonClient } from "../daemon/client";
import { DownloadTokenStore } from "../downloads/token_store";
import { safeHandleCommand, type GatewayDependencies } from "../index";
import { SessionStore } from "../session/store";
import { renderDiscordResponse } from "./renderers.discord";

function buildDeps(): GatewayDependencies {
  return {
    daemon: new InMemoryDaemonClient(),
    sessions: new SessionStore(),
    downloads: new DownloadTokenStore(),
  };
}

describe("renderDiscordResponse", () => {
  test("renders success response", () => {
    const response: GatewayResponse = {
      requestId: "req-1",
      result: "ok",
      message: "install completed for openclaw",
    };
    const rendered = renderDiscordResponse(response);
    expect(rendered.content).toBe("✅ install completed for openclaw");
  });

  test("renders error response with code", () => {
    const response: GatewayResponse = {
      requestId: "req-2",
      result: "error",
      errorCode: "E_NOT_INSTALLED",
      message: "agent is not installed",
    };
    const rendered = renderDiscordResponse(response);
    expect(rendered.content).toBe("❌ E_NOT_INSTALLED: agent is not installed");
  });

  test("renders download and handoff metadata lines", () => {
    const response: GatewayResponse = {
      requestId: "req-3",
      result: "ok",
      message: "remote diagnosis consent recorded",
      handoffId: "handoff-22",
      handoffStatus: "pending",
      downloadUrl: "/downloads/dl-22/openclaw.zip",
    };
    const rendered = renderDiscordResponse(response);
    expect(rendered.content).toContain("Download: /downloads/dl-22/openclaw.zip");
    expect(rendered.content).toContain("Handoff: handoff-22 (pending)");
  });
});

describe("discord renderer integration path", () => {
  test("pair and agents responses render for discord", async () => {
    const deps = buildDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("pair-ok");
    }

    const pair = await safeHandleCommand("discord chan-1 req-pair /pair pair-ok", deps);
    expect(pair.result).toBe("ok");

    const token = pair.sessionToken!;
    const agents = await safeHandleCommand(`discord chan-1 req-agents ${token} /agents`, deps);
    expect(agents.result).toBe("ok");

    const rendered = renderDiscordResponse(agents);
    expect(rendered.content).toContain("✅");
    expect(rendered.content).toContain("listed");
  });
});
