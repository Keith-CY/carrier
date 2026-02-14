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

describe("parser edge cases: mention stripping", () => {
  test("Discord: handles multiple consecutive mentions before command", () => {
    const parsed = parseDiscordPayloadToCommand({
      id: "edge-1",
      channel_id: "channel-100",
      content: "<@12345> <@!67890> <@99999> /agents",
    });

    expect(parsed?.command).toBe("/agents");
    expect(parsed?.args).toEqual([]);
  });

  test("Discord: handles mixed mention formats with text mentions", () => {
    const parsed = parseDiscordPayloadToCommand({
      id: "edge-2",
      channel_id: "channel-101",
      content: "<@!111> @bot-mention @another /status openclaw",
    });

    expect(parsed?.command).toBe("/status");
    expect(parsed?.args).toEqual(["openclaw"]);
  });

  test("Feishu: strips multiple consecutive @-style mentions", () => {
    const parsed = parseFeishuEventToCommand({
      header: { event_id: "evt-edge-1" },
      event: {
        message: {
          message_id: "msg-edge-1",
          chat_id: "oc_chat_100",
          content: JSON.stringify({ text: "@_user_1 @_user_2 @bot /upgrade agent-x" }),
        },
      },
    });

    expect(parsed?.command).toBe("/upgrade");
    expect(parsed?.args).toEqual(["agent-x"]);
  });

  test("Telegram: preserves mentions in args after command", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 2001,
      message: {
        message_id: 901,
        chat: { id: "chat-edge-1" },
        text: "/notify @user123 hello",
      },
    });

    expect(parsed?.command).toBe("/notify");
    expect(parsed?.args).toEqual(["@user123", "hello"]);
  });
});

describe("parser edge cases: bot suffix normalization", () => {
  test("Telegram: normalizes mixed-case bot suffix", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 2002,
      message: {
        message_id: 902,
        chat: { id: "chat-edge-2" },
        text: "/StAtUs@CaRrIeRbOt",
      },
    });

    expect(parsed?.command).toBe("/status");
    expect(parsed?.args).toEqual([]);
  });

  test("Telegram: strips bot suffix with dots and dashes", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 2003,
      message: {
        message_id: 903,
        chat: { id: "chat-edge-3" },
        text: "/agents@carrier-bot.prod agents",
      },
    });

    expect(parsed?.command).toBe("/agents");
    expect(parsed?.args).toEqual(["agents"]);
  });

  test("Discord: normalizes command name to lowercase", () => {
    const parsed = parseDiscordPayloadToCommand({
      id: "edge-3",
      channel_id: "channel-102",
      content: "/LOGS openclaw 100",
    });

    expect(parsed?.command).toBe("/logs");
    expect(parsed?.args).toEqual(["openclaw", "100"]);
  });
});

describe("parser edge cases: command-like prefixes that should fail", () => {
  test("returns null for standalone slash", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 3001,
      message: {
        message_id: 1001,
        chat: { id: "chat-fail-1" },
        text: "/",
      },
    });
    expect(parsed).toBeNull();
  });

  test("returns null for slash with only whitespace", () => {
    const parsed = parseDiscordPayloadToCommand({
      id: "fail-1",
      channel_id: "channel-200",
      content: "/   ",
    });
    expect(parsed).toBeNull();
  });

  test("returns null for only mentions without command", () => {
    const parsed = parseDiscordPayloadToCommand({
      id: "fail-2",
      channel_id: "channel-201",
      content: "<@12345> <@!67890>",
    });
    expect(parsed).toBeNull();
  });

  test("returns null for mentions followed by non-command text", () => {
    const parsed = parseFeishuEventToCommand({
      header: { event_id: "evt-fail-1" },
      event: {
        message: {
          message_id: "msg-fail-1",
          chat_id: "oc_chat_200",
          content: JSON.stringify({ text: "@_user_1 hello there" }),
        },
      },
    });
    expect(parsed).toBeNull();
  });

  test("returns null for text starting with slash but not a command", () => {
    const parsed = parseDiscordPayloadToCommand({
      id: "fail-3",
      channel_id: "channel-202",
      content: "/ not a command",
    });
    expect(parsed).toBeNull();
  });
});
