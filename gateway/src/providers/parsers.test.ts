import { describe, expect, test } from "bun:test";
import { generateKeyPairSync, sign } from "node:crypto";
import {
  parseDiscordPayloadToCommand,
  parseFeishuEventToCommand,
  parseTelegramUpdateToCommand,
  verifyTelegramWebhookSecret,
  verifyDiscordRequestSignature,
  verifyFeishuEventToken,
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

describe("verifyTelegramWebhookSecret", () => {
  test("allows requests when expected secret is not configured", () => {
    expect(verifyTelegramWebhookSecret(null, undefined)).toBe(true);
    expect(verifyTelegramWebhookSecret("anything", "")).toBe(true);
    expect(verifyTelegramWebhookSecret("anything", "   ")).toBe(true);
  });

  test("rejects requests without secret when expected secret is configured", () => {
    expect(verifyTelegramWebhookSecret(null, "shared-secret")).toBe(false);
    expect(verifyTelegramWebhookSecret("", "shared-secret")).toBe(false);
  });

  test("accepts exact secret token", () => {
    expect(verifyTelegramWebhookSecret("shared-secret", "shared-secret")).toBe(true);
  });

  test("trims surrounding whitespace before compare", () => {
    expect(verifyTelegramWebhookSecret(" shared-secret ", "shared-secret")).toBe(true);
    expect(verifyTelegramWebhookSecret("shared-secret", " shared-secret ")).toBe(true);
  });

  test("rejects near-miss secret token", () => {
    expect(verifyTelegramWebhookSecret("shared-secret-x", "shared-secret")).toBe(false);
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

describe("verifyDiscordRequestSignature", () => {
  function rawPublicKeyHexFromSpkiDer(der: Buffer): string {
    return der.subarray(der.length - 32).toString("hex");
  }

  test("accepts valid signature within timestamp window", () => {
    const { publicKey, privateKey } = generateKeyPairSync("ed25519");
    const body = JSON.stringify({ type: 2, data: { name: "status" } });
    const now = Math.floor(Date.now() / 1000);
    const timestamp = String(now);
    const signatureHex = sign(null, Buffer.from(`${timestamp}${body}`), privateKey).toString("hex");
    const publicKeyDer = publicKey.export({ type: "spki", format: "der" }) as Buffer;
    const publicKeyHex = rawPublicKeyHexFromSpkiDer(publicKeyDer);

    expect(verifyDiscordRequestSignature({
      body,
      timestamp,
      signatureHex,
      publicKeyHex,
      nowEpochSeconds: now,
    })).toBe(true);
  });

  test("rejects invalid signature", () => {
    const { publicKey, privateKey } = generateKeyPairSync("ed25519");
    const body = JSON.stringify({ type: 2, data: { name: "agents" } });
    const now = Math.floor(Date.now() / 1000);
    const timestamp = String(now);
    const validSig = sign(null, Buffer.from(`${timestamp}${body}`), privateKey).toString("hex");
    const invalidSig = `${validSig.slice(0, -2)}aa`;
    const publicKeyDer = publicKey.export({ type: "spki", format: "der" }) as Buffer;
    const publicKeyHex = rawPublicKeyHexFromSpkiDer(publicKeyDer);

    expect(verifyDiscordRequestSignature({
      body,
      timestamp,
      signatureHex: invalidSig,
      publicKeyHex,
      nowEpochSeconds: now,
    })).toBe(false);
  });

  test("rejects stale timestamp", () => {
    const { publicKey, privateKey } = generateKeyPairSync("ed25519");
    const body = JSON.stringify({ type: 2, data: { name: "logs" } });
    const now = Math.floor(Date.now() / 1000);
    const staleTimestamp = String(now - 1000);
    const signatureHex = sign(null, Buffer.from(`${staleTimestamp}${body}`), privateKey).toString("hex");
    const publicKeyDer = publicKey.export({ type: "spki", format: "der" }) as Buffer;
    const publicKeyHex = rawPublicKeyHexFromSpkiDer(publicKeyDer);

    expect(verifyDiscordRequestSignature({
      body,
      timestamp: staleTimestamp,
      signatureHex,
      publicKeyHex,
      nowEpochSeconds: now,
      maxAgeSeconds: 300,
    })).toBe(false);
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

describe("verifyFeishuEventToken", () => {
  test("allows events when expected token is not configured", () => {
    expect(verifyFeishuEventToken({}, undefined)).toBe(true);
    expect(verifyFeishuEventToken({}, "")).toBe(true);
  });

  test("accepts matching token from event header", () => {
    const payload = {
      header: {
        token: "verify-token-1",
      },
    };
    expect(verifyFeishuEventToken(payload, "verify-token-1")).toBe(true);
  });

  test("accepts matching token from root token field", () => {
    const payload = {
      token: "verify-token-2",
    };
    expect(verifyFeishuEventToken(payload, "verify-token-2")).toBe(true);
  });

  test("rejects missing or mismatched token", () => {
    expect(verifyFeishuEventToken({}, "verify-token-3")).toBe(false);
    expect(verifyFeishuEventToken({ header: { token: "wrong-token" } }, "verify-token-3")).toBe(false);
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


describe("Feishu edge cases: T021 normalization", () => {
  test("URL verification challenge event returns null", () => {
    const parsed = parseFeishuEventToCommand({
      type: "url_verification",
      challenge: "test-challenge-token-12345",
      token: "verification-token",
    });
    expect(parsed).toBeNull();
  });

  test("non-text message (image) returns null", () => {
    const parsed = parseFeishuEventToCommand({
      header: { event_id: "evt-img-1" },
      event: {
        message: {
          message_id: "msg-img-1",
          chat_id: "oc_chat_img",
          message_type: "image",
          content: JSON.stringify({ image_key: "img_v2_12345" }),
        },
      },
    });
    expect(parsed).toBeNull();
  });

  test("non-text message (file) returns null", () => {
    const parsed = parseFeishuEventToCommand({
      header: { event_id: "evt-file-1" },
      event: {
        message: {
          message_id: "msg-file-1",
          chat_id: "oc_chat_file",
          message_type: "file",
          content: JSON.stringify({ file_key: "file_v2_67890" }),
        },
      },
    });
    expect(parsed).toBeNull();
  });

  test("non-text message (post/rich text) returns null", () => {
    const parsed = parseFeishuEventToCommand({
      header: { event_id: "evt-post-1" },
      event: {
        message: {
          message_id: "msg-post-1",
          chat_id: "oc_chat_post",
          message_type: "post",
          content: JSON.stringify({
            post: {
              zh_cn: {
                title: "Title",
                content: [["Some rich text"]],
              },
            },
          }),
        },
      },
    });
    expect(parsed).toBeNull();
  });

  test("group mention with @bot prefix strips and parses command", () => {
    const parsed = parseFeishuEventToCommand({
      header: { event_id: "evt-mention-1" },
      event: {
        message: {
          message_id: "msg-mention-1",
          chat_id: "oc_chat_group",
          content: JSON.stringify({ text: "@_user_1234567890 /status openclaw" }),
        },
      },
    });
    expect(parsed).not.toBeNull();
    expect(parsed?.command).toBe("/status");
    expect(parsed?.args).toEqual(["openclaw"]);
  });

  test("empty text content returns null", () => {
    const parsed = parseFeishuEventToCommand({
      header: { event_id: "evt-empty-1" },
      event: {
        message: {
          message_id: "msg-empty-1",
          chat_id: "oc_chat_empty",
          content: JSON.stringify({ text: "" }),
        },
      },
    });
    expect(parsed).toBeNull();
  });

  test("whitespace-only text content returns null", () => {
    const parsed = parseFeishuEventToCommand({
      header: { event_id: "evt-ws-1" },
      event: {
        message: {
          message_id: "msg-ws-1",
          chat_id: "oc_chat_ws",
          content: JSON.stringify({ text: "   \n\t  " }),
        },
      },
    });
    expect(parsed).toBeNull();
  });

  test("edited message with valid command is handled correctly", () => {
    // Feishu edited messages may still arrive as message events
    // The parser should handle them the same way as new messages
    const parsed = parseFeishuEventToCommand({
      header: { event_id: "evt-edit-1" },
      event: {
        message: {
          message_id: "msg-edit-1",
          chat_id: "oc_chat_edit",
          content: JSON.stringify({ text: "/agents" }),
          edit_time: "1234567890",
        },
      },
    });
    expect(parsed).not.toBeNull();
    expect(parsed?.command).toBe("/agents");
    expect(parsed?.args).toEqual([]);
  });

  test("mention-only text after stripping returns null", () => {
    const parsed = parseFeishuEventToCommand({
      header: { event_id: "evt-mention-only" },
      event: {
        message: {
          message_id: "msg-mention-only",
          chat_id: "oc_chat_mention",
          content: JSON.stringify({ text: "@_user_1 @_user_2" }),
        },
      },
    });
    expect(parsed).toBeNull();
  });

  test("multiple mentions before command are all stripped", () => {
    const parsed = parseFeishuEventToCommand({
      header: { event_id: "evt-multi-mention" },
      event: {
        message: {
          message_id: "msg-multi-mention",
          chat_id: "oc_chat_multi",
          content: JSON.stringify({ text: "@_bot @_user_a @_user_b /upgrade agent-prod" }),
        },
      },
    });
    expect(parsed).not.toBeNull();
    expect(parsed?.command).toBe("/upgrade");
    expect(parsed?.args).toEqual(["agent-prod"]);
  });
});


describe("Telegram edge cases (T019): non-message updates", () => {
  test("returns null for callback_query update", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 4001,
      callback_query: {
        id: "callback-1",
        from: { id: 123456 },
        message: {
          message_id: 999,
          chat: { id: 123456 },
          text: "/status",
        },
        data: "button_pressed",
      },
    });
    expect(parsed).toBeNull();
  });

  test("returns null for inline_query update", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 4002,
      inline_query: {
        id: "inline-1",
        from: { id: 123456 },
        query: "/agents",
      },
    });
    expect(parsed).toBeNull();
  });

  test("returns null for update with no message-like fields", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 4003,
      my_chat_member: {
        chat: { id: 123456 },
        from: { id: 789 },
      },
    });
    expect(parsed).toBeNull();
  });
});

describe("Telegram edge cases (T019): non-text messages", () => {
  test("returns null for photo message without caption", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 5001,
      message: {
        message_id: 1100,
        chat: { id: 123456 },
        photo: [
          { file_id: "photo-1", width: 100, height: 100 },
        ],
      },
    });
    expect(parsed).toBeNull();
  });

  test("parses photo message with command in caption", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 5002,
      message: {
        message_id: 1101,
        chat: { id: 123456 },
        photo: [
          { file_id: "photo-2", width: 100, height: 100 },
        ],
        caption: "/analyze image",
      },
    });
    expect(parsed?.command).toBe("/analyze");
    expect(parsed?.args).toEqual(["image"]);
  });

  test("returns null for sticker message", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 5003,
      message: {
        message_id: 1102,
        chat: { id: 123456 },
        sticker: {
          file_id: "sticker-1",
          width: 512,
          height: 512,
        },
      },
    });
    expect(parsed).toBeNull();
  });

  test("returns null for video message without caption", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 5004,
      message: {
        message_id: 1103,
        chat: { id: 123456 },
        video: {
          file_id: "video-1",
          width: 1920,
          height: 1080,
        },
      },
    });
    expect(parsed).toBeNull();
  });

  test("returns null for document message without caption", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 5005,
      message: {
        message_id: 1104,
        chat: { id: 123456 },
        document: {
          file_id: "doc-1",
          file_name: "report.pdf",
        },
      },
    });
    expect(parsed).toBeNull();
  });

  test("returns null for audio message", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 5006,
      message: {
        message_id: 1105,
        chat: { id: 123456 },
        audio: {
          file_id: "audio-1",
          duration: 180,
        },
      },
    });
    expect(parsed).toBeNull();
  });

  test("returns null for voice message", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 5007,
      message: {
        message_id: 1106,
        chat: { id: 123456 },
        voice: {
          file_id: "voice-1",
          duration: 5,
        },
      },
    });
    expect(parsed).toBeNull();
  });

  test("returns null for location message", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 5008,
      message: {
        message_id: 1107,
        chat: { id: 123456 },
        location: {
          latitude: 37.7749,
          longitude: -122.4194,
        },
      },
    });
    expect(parsed).toBeNull();
  });
});

describe("Telegram edge cases (T019): empty and whitespace text", () => {
  test("returns null for empty text string", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 6001,
      message: {
        message_id: 1200,
        chat: { id: 123456 },
        text: "",
      },
    });
    expect(parsed).toBeNull();
  });

  test("returns null for whitespace-only text", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 6002,
      message: {
        message_id: 1201,
        chat: { id: 123456 },
        text: "   \t\n  ",
      },
    });
    expect(parsed).toBeNull();
  });

  test("returns null for message with only bot mention", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 6003,
      message: {
        message_id: 1202,
        chat: { id: 123456 },
        text: "@CarrierBot",
      },
    });
    expect(parsed).toBeNull();
  });

  test("returns null for whitespace after stripping mentions", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 6004,
      message: {
        message_id: 1203,
        chat: { id: 123456 },
        text: "@bot1 @bot2   ",
      },
    });
    expect(parsed).toBeNull();
  });
});

describe("Telegram edge cases (T019): edited messages", () => {
  test("parses edited_message with command", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 7001,
      edited_message: {
        message_id: 1300,
        chat: { id: 123456 },
        text: "/status updated",
        edit_date: 1234567890,
      },
    });
    expect(parsed?.command).toBe("/status");
    expect(parsed?.args).toEqual(["updated"]);
    expect(parsed?.requestId).toBe("tg-7001-1300");
  });

  test("parses channel_post with command", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 7002,
      channel_post: {
        message_id: 1301,
        chat: { id: -100123456789, type: "channel" },
        text: "/broadcast message",
      },
    });
    expect(parsed?.command).toBe("/broadcast");
    expect(parsed?.args).toEqual(["message"]);
  });

  test("parses edited_channel_post with command", () => {
    const parsed = parseTelegramUpdateToCommand({
      update_id: 7003,
      edited_channel_post: {
        message_id: 1302,
        chat: { id: -100123456789, type: "channel" },
        text: "/announce edited",
        edit_date: 1234567890,
      },
    });
    expect(parsed?.command).toBe("/announce");
    expect(parsed?.args).toEqual(["edited"]);
  });
});
