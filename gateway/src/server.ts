import { type GatewayDependencies, handleCommand, parseInput, ParseError } from "./index";
import type { GatewayCommand, GatewayResponse } from "./contracts/commands";

const DEFAULT_PORT = 7332;

export type ServerConfig = {
  port?: number;
  deps: GatewayDependencies;
};

/**
 * Extract command text from a Telegram webhook update.
 */
function parseTelegramUpdate(body: unknown): { chatId: string; text: string } | null {
  const update = body as Record<string, unknown>;
  const message = update?.message as Record<string, unknown> | undefined;
  if (!message) return null;
  const chat = message.chat as Record<string, unknown> | undefined;
  const chatId = String(chat?.id ?? "");
  const text = String(message.text ?? "");
  if (!chatId || !text) return null;
  return { chatId, text };
}

/**
 * Extract command text from a Discord webhook message.
 */
function parseDiscordMessage(body: unknown): { chatId: string; text: string } | null {
  const msg = body as Record<string, unknown>;
  const channelId = String(msg?.channel_id ?? "");
  const content = String(msg?.content ?? "");
  if (!channelId || !content) return null;
  return { chatId: channelId, text: content };
}

/**
 * Extract command text from a Feishu event callback.
 */
function parseFeishuEvent(body: unknown): { chatId: string; text: string } | null {
  const payload = body as Record<string, unknown>;
  const event = payload?.event as Record<string, unknown> | undefined;
  const message = event?.message as Record<string, unknown> | undefined;
  const chatId = String(message?.chat_id ?? "");
  const content = message?.content as string | undefined;
  if (!chatId || !content) return null;

  // Feishu sends content as JSON: {"text":"..."}
  let text = content;
  try {
    const parsed = JSON.parse(content);
    text = parsed.text ?? content;
  } catch {
    // use raw
  }
  return { chatId, text };
}

function buildGatewayInput(provider: string, chatId: string, text: string): string {
  // text should be the command like "/install openclaw"
  // We generate a requestId
  const requestId = `req-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  return `${provider} ${chatId} ${requestId} ${text}`;
}

async function handleWebhook(
  provider: string,
  parsed: { chatId: string; text: string } | null,
  deps: GatewayDependencies,
): Promise<Response> {
  if (!parsed) {
    return Response.json({ error: "could not parse webhook body" }, { status: 400 });
  }

  const { chatId, text } = parsed;
  if (!text.startsWith("/")) {
    // Not a command, acknowledge
    return Response.json({ ok: true });
  }

  const input = buildGatewayInput(provider, chatId, text);
  let response: GatewayResponse;
  try {
    const cmd = parseInput(input);
    response = await handleCommand(cmd, deps);
  } catch (err) {
    if (err instanceof ParseError) {
      response = {
        requestId: err.requestId,
        result: "error",
        errorCode: "E_PARSE",
        message: err.message,
      };
    } else {
      response = {
        requestId: "unknown",
        result: "error",
        errorCode: "E_INTERNAL",
        message: err instanceof Error ? err.message : "unknown error",
      };
    }
  }

  return Response.json(response);
}

export function createServer(config: ServerConfig) {
  const port = config.port ?? (Number(process.env.CARRIER_GATEWAY_PORT) || DEFAULT_PORT);
  const deps = config.deps;

  const server = Bun.serve({
    port,
    async fetch(req) {
      const url = new URL(req.url);
      const path = url.pathname;

      // Health check
      if (path === "/healthz" && req.method === "GET") {
        return new Response("ok");
      }

      // Webhook routes
      if (req.method === "POST") {
        const body = await req.json().catch(() => null);
        if (!body) {
          return Response.json({ error: "invalid JSON body" }, { status: 400 });
        }

        if (path === "/webhook/telegram") {
          return handleWebhook("telegram", parseTelegramUpdate(body), deps);
        }
        if (path === "/webhook/discord") {
          return handleWebhook("discord", parseDiscordMessage(body), deps);
        }
        if (path === "/webhook/feishu") {
          // Handle Feishu URL verification challenge
          const payload = body as Record<string, unknown>;
          if (payload.challenge) {
            return Response.json({ challenge: payload.challenge });
          }
          return handleWebhook("feishu", parseFeishuEvent(body), deps);
        }
      }

      // Download route
      if (path.startsWith("/download/") && req.method === "GET") {
        const token = path.slice("/download/".length);
        if (!token) {
          return Response.json({ error: "missing token" }, { status: 400 });
        }
        const record = deps.downloads.consume(token);
        if (!record) {
          return Response.json({ error: "token invalid or expired" }, { status: 404 });
        }
        // In production, stream the file. For now, return the reference.
        return Response.json({
          fileRef: record.fileRef,
          message: "download ready",
        });
      }

      return Response.json({ error: "not found" }, { status: 404 });
    },
  });

  return server;
}
