import { describe, expect, test } from "bun:test";
import { InMemoryDaemonClient } from "../daemon/client";
import { DownloadTokenStore } from "../downloads/token_store";
import { safeHandleCommand, type GatewayDependencies } from "../index";
import { SessionStore } from "../session/store";
import {
  parseDiscordPayloadToCommand,
  parseFeishuEventToCommand,
  parseTelegramUpdateToCommand,
  toGatewayInput,
} from "./parsers";

function buildDeps(): GatewayDependencies {
  return {
    daemon: new InMemoryDaemonClient(),
    sessions: new SessionStore(),
    downloads: new DownloadTokenStore(),
  };
}

describe("provider parser e2e", () => {
  test("telegram parser output is executable by gateway command path", async () => {
    const deps = buildDeps();
    deps.sessions.registerPairCode("tg-code", 300);

    const normalized = parseTelegramUpdateToCommand({
      update_id: 2001,
      message: {
        message_id: 3001,
        chat: { id: "tg-chat" },
        text: "/pair tg-code",
      },
    });
    expect(normalized).not.toBeNull();

    const res = await safeHandleCommand(toGatewayInput(normalized!), deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("paired");
  });

  test("discord parser output is executable by gateway command path", async () => {
    const deps = buildDeps();
    deps.sessions.registerPairCode("dc-code", 300);

    const normalized = parseDiscordPayloadToCommand({
      id: "interaction-22",
      type: 2,
      channel_id: "dc-chan",
      data: {
        name: "pair",
        options: [{ name: "code", value: "dc-code" }],
      },
    });
    expect(normalized).not.toBeNull();

    const res = await safeHandleCommand(toGatewayInput(normalized!), deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("paired");
  });

  test("feishu parser output is executable by gateway command path", async () => {
    const deps = buildDeps();
    deps.sessions.registerPairCode("fs-code", 300);

    const normalized = parseFeishuEventToCommand({
      header: { event_id: "evt-20" },
      event: {
        message: {
          message_id: "msg-20",
          chat_id: "fs-chat",
          content: JSON.stringify({ text: "/pair fs-code" }),
        },
      },
    });
    expect(normalized).not.toBeNull();

    const res = await safeHandleCommand(toGatewayInput(normalized!), deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("paired");
  });
});
