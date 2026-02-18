import type { Provider } from "../contracts/commands";
import { createHash, createPublicKey, timingSafeEqual, verify } from "node:crypto";

/** ASN.1 DER prefix for wrapping a raw 32-byte Ed25519 public key into SPKI format. */
const ED25519_SPKI_PREFIX = "302a300506032b6570032100";

export type NormalizedGatewayCommand = {
  provider: Provider;
  chatId: string;
  requestId: string;
  command: string;
  args: string[];
  rawText: string;
};

export function toGatewayInput(command: NormalizedGatewayCommand): string {
  return [
    command.provider,
    command.chatId,
    command.requestId,
    command.command,
    ...command.args,
  ].join(" ");
}

export function parseTelegramUpdateToCommand(payload: unknown): NormalizedGatewayCommand | null {
  const update = asRecord(payload);
  if (!update) {
    return null;
  }

  const message =
    asRecord(update.message) ??
    asRecord(update.edited_message) ??
    asRecord(update.channel_post) ??
    asRecord(update.edited_channel_post);
  if (!message) {
    return null;
  }

  const chat = asRecord(message.chat);
  const chatId = toID(chat?.id);
  const rawText = firstString(message.text, message.caption);
  if (!chatId || !rawText) {
    return null;
  }

  const parsed = parseCommandText(rawText);
  if (!parsed) {
    return null;
  }

  const requestId = buildRequestId("tg", toID(update.update_id), toID(message.message_id));
  return {
    provider: "telegram",
    chatId,
    requestId,
    command: parsed.command,
    args: parsed.args,
    rawText,
  };
}

export function verifyTelegramWebhookSecret(
  providedSecret: string | null | undefined,
  expectedSecret: string | null | undefined,
): boolean {
  const expected = expectedSecret?.trim() ?? "";
  if (expected.length === 0) {
    return true;
  }

  const provided = providedSecret?.trim() ?? "";
  if (provided.length === 0) {
    return false;
  }

  return constantTimeStringEquals(provided, expected);
}

export function parseDiscordPayloadToCommand(payload: unknown): NormalizedGatewayCommand | null {
  const data = asRecord(payload);
  if (!data) {
    return null;
  }

  // Slash-command interaction payload
  if (toNumber(data.type) === 2) {
    const interaction = asRecord(data.data);
    const commandName = firstString(interaction?.name);
    if (!commandName) {
      return null;
    }

    const options = flattenDiscordOptionValues(interaction?.options);
    const chatId = toID(data.channel_id) ?? toID(data.guild_id);
    if (!chatId) {
      return null;
    }

    const requestId = buildRequestId("dc", toID(data.id));
    return {
      provider: "discord",
      chatId,
      requestId,
      command: normalizeCommandName(`/${commandName}`),
      args: options,
      rawText: `/${commandName} ${options.join(" ")}`.trim(),
    };
  }

  // Message-create style payload
  const content = firstString(data.content);
  const chatId = toID(data.channel_id) ?? toID(data.guild_id);
  if (!content || !chatId) {
    return null;
  }
  const parsed = parseCommandText(content);
  if (!parsed) {
    return null;
  }

  const requestId = buildRequestId("dc", toID(data.id), toID(asRecord(data.interaction)?.id));
  return {
    provider: "discord",
    chatId,
    requestId,
    command: parsed.command,
    args: parsed.args,
    rawText: content,
  };
}

export type DiscordSignatureVerificationInput = {
  body: string;
  signatureHex: string | null | undefined;
  timestamp: string | null | undefined;
  publicKeyHex: string | null | undefined;
  maxAgeSeconds?: number;
  nowEpochSeconds?: number;
};

export function verifyDiscordRequestSignature(input: DiscordSignatureVerificationInput): boolean {
  const signatureHex = input.signatureHex?.trim() ?? "";
  const timestamp = input.timestamp?.trim() ?? "";
  const publicKeyHex = input.publicKeyHex?.trim() ?? "";
  if (!signatureHex || !timestamp || !publicKeyHex) {
    return false;
  }

  const maxAgeSeconds = input.maxAgeSeconds ?? 300;
  const nowEpochSeconds = input.nowEpochSeconds ?? Math.floor(Date.now() / 1000);
  const ts = Number.parseInt(timestamp, 10);
  if (!Number.isFinite(ts) || Math.abs(nowEpochSeconds - ts) > maxAgeSeconds) {
    return false;
  }

  try {
    const signature = Buffer.from(signatureHex, "hex");
    const publicKeyRaw = Buffer.from(publicKeyHex, "hex");
    if (signature.length !== 64 || publicKeyRaw.length !== 32) {
      return false;
    }

    // Discord public keys are raw 32-byte Ed25519 keys. Node verify() expects SPKI.
    const spkiPrefix = Buffer.from(ED25519_SPKI_PREFIX, "hex");
    const publicKey = createPublicKey({
      key: Buffer.concat([spkiPrefix, publicKeyRaw]),
      format: "der",
      type: "spki",
    });

    return verify(
      null,
      Buffer.from(`${timestamp}${input.body}`),
      publicKey,
      signature,
    );
  } catch {
    return false;
  }
}

export function parseFeishuEventToCommand(payload: unknown): NormalizedGatewayCommand | null {
  const root = asRecord(payload);
  if (!root) {
    return null;
  }
  if (firstString(root.type) === "url_verification") {
    return null;
  }

  const header = asRecord(root.header);
  const event = asRecord(root.event);
  const message = asRecord(event?.message);
  if (!message) {
    return null;
  }

  const chatId = toID(message.chat_id) ?? toID(message.open_chat_id);
  if (!chatId) {
    return null;
  }

  const rawText = parseFeishuTextContent(message.content);
  if (!rawText) {
    return null;
  }

  const parsed = parseCommandText(rawText);
  if (!parsed) {
    return null;
  }

  const requestId = buildRequestId("fs", toID(header?.event_id), toID(message.message_id), toID(root.uuid));
  return {
    provider: "feishu",
    chatId,
    requestId,
    command: parsed.command,
    args: parsed.args,
    rawText,
  };
}

export function verifyFeishuEventToken(
  payload: unknown,
  expectedToken: string | null | undefined,
): boolean {
  const expected = expectedToken?.trim() ?? "";
  if (expected.length === 0) {
    return true;
  }

  const root = asRecord(payload);
  if (!root) {
    return false;
  }

  const header = asRecord(root.header);
  const token = firstString(header?.token, root.token);
  if (!token) {
    return false;
  }

  return constantTimeStringEquals(token.trim(), expected);
}

type ParsedCommandText = {
  command: string;
  args: string[];
};

function parseCommandText(rawText: string): ParsedCommandText | null {
  const normalized = stripLeadingMentions(rawText).trim();
  if (!normalized) {
    return null;
  }

  const parts = normalized.split(/\s+/);
  if (parts.length === 0) {
    return null;
  }

  const rawCommand = normalizeCommandName(parts[0] ?? "");
  if (!rawCommand.startsWith("/") || rawCommand === "/") {
    return null;
  }

  return {
    command: rawCommand,
    args: parts.slice(1),
  };
}

function stripLeadingMentions(input: string): string {
  return input
    .replace(/^(?:(?:<@!?[0-9]+>|@\S+)\s*)+/, "")
    .trim();
}

function normalizeCommandName(input: string): string {
  const lowered = input.trim().toLowerCase();
  return lowered.replace(/@[\w.-]+$/, "");
}

function flattenDiscordOptionValues(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }

  const out: string[] = [];
  for (const item of value) {
    const option = asRecord(item);
    if (!option) {
      continue;
    }

    if ("value" in option) {
      out.push(String(option.value));
      continue;
    }

    out.push(...flattenDiscordOptionValues(option.options));
  }
  return out;
}

function parseFeishuTextContent(raw: unknown): string | null {
  if (typeof raw === "string") {
    try {
      const decoded = JSON.parse(raw) as Record<string, unknown>;
      if (typeof decoded.text === "string") {
        return decoded.text;
      }
      return null;
    } catch {
      return raw;
    }
  }

  const direct = asRecord(raw);
  if (typeof direct?.text === "string") {
    return direct.text;
  }
  return null;
}

function buildRequestId(prefix: string, ...parts: Array<string | null>): string {
  const compact = parts.filter((value) => value && value.trim().length > 0);
  if (compact.length === 0) {
    return `${prefix}-${crypto.randomUUID()}`;
  }
  return `${prefix}-${compact.join("-")}`;
}

function firstString(...values: unknown[]): string | null {
  for (const value of values) {
    if (typeof value === "string" && value.trim().length > 0) {
      return value;
    }
  }
  return null;
}

function toNumber(value: unknown): number | null {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string" && value.trim().length > 0) {
    const parsed = Number.parseFloat(value);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }
  return null;
}

function toID(value: unknown): string | null {
  if (typeof value === "string" && value.trim().length > 0) {
    return value.trim();
  }
  if (typeof value === "number" && Number.isFinite(value)) {
    return String(value);
  }
  return null;
}

export function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

/**
 * Constant-time string comparison that does not leak length information.
 * Both inputs are hashed before comparison to normalize length.
 */
function constantTimeStringEquals(left: string, right: string): boolean {
  const leftHash = createHash("sha256").update(left, "utf8").digest();
  const rightHash = createHash("sha256").update(right, "utf8").digest();
  return timingSafeEqual(leftHash, rightHash);
}
