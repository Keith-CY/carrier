import { describe, expect, test } from "bun:test";
import {
  parseDiscordPayloadToCommand,
  parseFeishuEventToCommand,
  parseTelegramUpdateToCommand,
  toGatewayInput,
} from "./parsers";

describe("parseTelegramUpdateToCommand", () => {
  test("parses basic command update", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 1001,
      message: {
        message_id: 777,
        chat: { id: 123456 },
        text: "/agents",
      },
    });

    expect(parsed?.provider).toBe("telegram");
    expect(parsed?.chatId).toBe("123456");
    expect(parsed?.command).toBe("/agents");
    expect(parsed?.args).toEqual([]);
    expect(parsed?.requestId).toBe("tg-1001-777");
  });

  test("strips telegram bot suffix from command", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 1002,
      message: {
        message_id: 778,
        chat: { id: "chat-1" },
        text: "/status@CarrierBot openclaw",
      },
    });

    expect(parsed?.command).toBe("/status");
    expect(parsed?.args).toEqual(["openclaw"]);
  });

  test("returns null when message text is not command", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 1003,
      message: {
        message_id: 779,
        chat: { id: "chat-2" },
        text: "hello",
      },
    });
    expect(parsed).toBeNull();
  });
});

describe("parseDiscordPayloadToCommand", () => {
  test("parses slash interaction payload with option values", () => {
    const parsed = parseDiscordPayloadToCommand({
      id: "interaction-1",
      type: 2,
      channel_id: "channel-9",
      data: {
        name: "upgrade",
        options: [
          { name: "agent", value: "openclaw" },
        ],
      },
    });

    expect(parsed?.provider).toBe("discord");
    expect(parsed?.chatId).toBe("channel-9");
    expect(parsed?.command).toBe("/upgrade");
    expect(parsed?.args).toEqual(["openclaw"]);
    expect(parsed?.requestId).toBe("dc-interaction-1");
  });

  test("parses message payload with leading mention", () => {
    const parsed = parseDiscordPayloadToCommand({
      id: "message-1",
      channel_id: "channel-7",
      content: "<@12345> /logs openclaw 50",
    });

    expect(parsed?.command).toBe("/logs");
    expect(parsed?.args).toEqual(["openclaw", "50"]);
  });

  test("parses message payload with mixed leading mentions", () => {
    const parsed = parseDiscordPayloadToCommand({
      id: "message-1b",
      channel_id: "channel-7",
      content: "<@!999> @carrier-bot /status",
    });

    expect(parsed?.command).toBe("/status");
    expect(parsed?.args).toEqual([]);
  });

  test("returns null when payload has no command text", () => {
    const parsed = parseDiscordPayloadToCommand({
      id: "message-2",
      channel_id: "channel-7",
      content: "just a text message",
    });
    expect(parsed).toBeNull();
  });
});

describe("parseFeishuEventToCommand", () => {
  test("parses text command from feishu message event", () => {
    const parsed = parseFeishuEventToCommand({
      header: { event_id: "evt-1" },
      event: {
        message: {
          message_id: "msg-1",
          chat_id: "oc_chat_1",
          content: JSON.stringify({ text: "/pair pair-code" }),
        },
      },
    });

    expect(parsed?.provider).toBe("feishu");
    expect(parsed?.chatId).toBe("oc_chat_1");
    expect(parsed?.command).toBe("/pair");
    expect(parsed?.args).toEqual(["pair-code"]);
    expect(parsed?.requestId).toBe("fs-evt-1-msg-1");
  });

  test("strips leading mention token in feishu text", () => {
    const parsed = parseFeishuEventToCommand({
      header: { event_id: "evt-2" },
      event: {
        message: {
          message_id: "msg-2",
          chat_id: "oc_chat_2",
          content: JSON.stringify({ text: "@_user_1 /agents" }),
        },
      },
    });

    expect(parsed?.command).toBe("/agents");
  });

  test("returns null for non-command feishu payload", () => {
    const parsed = parseFeishuEventToCommand({
      type: "url_verification",
      challenge: "challenge-token",
    });
    expect(parsed).toBeNull();
  });
});

describe("toGatewayInput", () => {
  test("renders normalized command into gateway command text", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 1004,
      message: {
        message_id: 780,
        chat: { id: 123 },
        text: "/diagnose openclaw",
      },
    });
    expect(parsed).not.toBeNull();
    expect(toGatewayInput(parsed!)).toBe("telegram 123 tg-1004-780 /diagnose openclaw");
  });
});
