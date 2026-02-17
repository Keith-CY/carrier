import { safeHandleCommand, type GatewayDependencies } from "./index";
import { HttpDaemonClient } from "./daemon/http_client";
import {
  buildContentDisposition,
  compareRequestedFileName,
  parseDownloadPath,
} from "./downloads/http";
import { DownloadTokenStore } from "./downloads/token_store";
import { RateLimiter } from "./ratelimit";
import { SessionStore } from "./session/store";
import { join } from "node:path";
import { timingSafeEqual } from "node:crypto";
import {
  asRecord,
  parseDiscordPayloadToCommand,
  parseFeishuEventToCommand,
  parseTelegramUpdateToCommand,
  toGatewayInput,
  verifyDiscordRequestSignature,
  verifyFeishuEventToken,
  verifyTelegramWebhookSecret,
} from "./providers/parsers";
import { renderDiscordResponse } from "./providers/renderers.discord";
import { renderFeishuResponse } from "./providers/renderers.feishu";
import { renderTelegramResponse } from "./providers/renderers";

export type GatewayRequestContext = {
  request: Request;
  requestId: string;
  deps: GatewayDependencies;
};

export type GatewayHandler = (ctx: GatewayRequestContext) => Promise<Response>;

export type GatewayMiddleware = (
  ctx: GatewayRequestContext,
  next: () => Promise<Response>,
) => Promise<Response>;

type ReadFileFn = (fileRef: string) => Promise<Blob | null>;

export type GatewayRuntimeOptions = {
  deps?: Partial<GatewayDependencies>;
  middlewares?: GatewayMiddleware[];
  readFile?: ReadFileFn;
  maxCommandBodyBytes?: number;
};

export type GatewayRuntime = {
  deps: GatewayDependencies;
  fetch: (request: Request) => Promise<Response>;
};

const DEFAULT_MAX_COMMAND_BODY_BYTES = 64 * 1024;

/** Cached at module load — env vars don't change at runtime. */
const cachedMaxCommandBodyBytes = loadMaxCommandBodyBytes();

class PayloadTooLargeError extends Error {
  constructor(readonly maxBytes: number) {
    super(`request body exceeds ${maxBytes} bytes`);
    this.name = "PayloadTooLargeError";
  }
}

export function composeMiddleware(middlewares: GatewayMiddleware[], handler: GatewayHandler): GatewayHandler {
  return async (ctx: GatewayRequestContext): Promise<Response> => {
    let index = -1;
    const dispatch = async (middlewareIndex: number): Promise<Response> => {
      if (middlewareIndex <= index) {
        throw new Error("next() called multiple times");
      }
      index = middlewareIndex;
      const middleware = middlewares[middlewareIndex];
      if (!middleware) {
        return handler(ctx);
      }
      return await middleware(ctx, () => dispatch(middlewareIndex + 1));
    };
    return await dispatch(0);
  };
}

export const requestIdMiddleware: GatewayMiddleware = async (ctx, next) => {
  const incoming = ctx.request.headers.get("x-request-id")?.trim().replace(/[\x00-\x1F\x7F]/g, "");
  ctx.requestId = incoming && incoming.length > 0 ? incoming : crypto.randomUUID();
  const response = await next();
  response.headers.set("x-request-id", ctx.requestId);
  return response;
};

export function createRuntimeDependencies(overrides: Partial<GatewayDependencies> = {}): GatewayDependencies {
  // Determine session persistence path from environment or use a default
  const dataDir = process.env.SESSION_DATA_DIR ?? process.env.ARTIFACT_ROOT ?? process.cwd();
  const sessionPersistencePath = join(dataDir, "sessions.json");
  
  return {
    daemon: overrides.daemon ?? new HttpDaemonClient(),
    sessions: overrides.sessions ?? new SessionStore(undefined, undefined, sessionPersistencePath).startPeriodicCleanup(),
    downloads: overrides.downloads ?? new DownloadTokenStore().startPeriodicCleanup(),
    rateLimiter: overrides.rateLimiter ?? new RateLimiter().startPeriodicCleanup(),
  };
}

const webhookHandlers: Map<string, (ctx: GatewayRequestContext) => Promise<Response>> = new Map([
  ["/webhook/discord", handleDiscordWebhookRequest],
  ["/webhook/feishu", handleFeishuWebhookRequest],
  ["/webhook/telegram", handleTelegramWebhookRequest],
]);

export function createGatewayRuntime(options: GatewayRuntimeOptions = {}): GatewayRuntime {
  const deps = createRuntimeDependencies(options.deps);
  const readFile = options.readFile ?? defaultReadFile;
  const middlewares = options.middlewares ?? [requestIdMiddleware];
  const maxBodyBytes = options.maxCommandBodyBytes ?? cachedMaxCommandBodyBytes;

  const router: GatewayHandler = async (ctx) => {
    const url = new URL(ctx.request.url);
    if (ctx.request.method === "GET" && url.pathname === "/healthz") {
      return jsonResponse({
        status: "ok",
      });
    }

    if (ctx.request.method === "POST" && url.pathname === "/command") {
      const gatewayAuthError = validateGatewayAPIToken(ctx.request, ctx.requestId);
      if (gatewayAuthError) {
        return jsonResponse(gatewayAuthError, 401);
      }

      let parsed: ParsedCommandRequest;
      try {
        parsed = await parseCommandRequest(ctx.request, maxBodyBytes);
      } catch (error) {
        if (error instanceof PayloadTooLargeError) {
          return jsonResponse({
            requestId: ctx.requestId,
            result: "error",
            errorCode: "E_PAYLOAD_TOO_LARGE",
            message: `request body exceeds ${error.maxBytes} bytes limit`,
          }, 413);
        }
        throw error;
      }
      if (!parsed.commandInput) {
        return jsonResponse({
          requestId: ctx.requestId,
          result: "error",
          errorCode: "E_USAGE",
          message: "request body must provide command input",
        }, 400);
      }
      
      // Validate authentication for non-/pair commands
      const authError = validateCommandAuth(parsed.commandInput, parsed.sessionToken, deps);
      if (authError) {
        return jsonResponse(authError, 401);
      }

      const commandInput = injectSessionTokenIfMissing(parsed.commandInput, parsed.sessionToken);
      const response = await safeHandleCommand(commandInput, deps);
      return jsonResponse(response);
    }

    if (ctx.request.method === "POST") {
      const webhookHandler = webhookHandlers.get(url.pathname);
      if (webhookHandler) {
        return await webhookHandler(ctx);
      }
    }

    if (ctx.request.method === "GET" && url.pathname.startsWith("/downloads/")) {
      return await handleDownloadRequest(ctx, url, readFile);
    }

    return jsonResponse({
      requestId: ctx.requestId,
      result: "error",
      errorCode: "E_NOT_FOUND",
      message: "route not found",
    }, 404);
  };

  const handler = composeMiddleware(middlewares, router);

  return {
    deps,
    fetch: async (request: Request) => await handler({
      request,
      requestId: "",
      deps,
    }),
  };
}

export type StartGatewayServerOptions = {
  port?: number;
  hostname?: string;
  deps?: Partial<GatewayDependencies>;
  middlewares?: GatewayMiddleware[];
};

export function startGatewayServer(options: StartGatewayServerOptions = {}): ReturnType<typeof Bun.serve> {
  const runtime = createGatewayRuntime({
    deps: options.deps,
    middlewares: options.middlewares,
  });
  const port = options.port ?? parsePort(process.env.CARRIER_GATEWAY_PORT, 8787);
  const hostname = options.hostname ?? process.env.CARRIER_GATEWAY_HOST ?? "127.0.0.1";
  if (!loadDiscordPublicKey()) {
    console.warn("[gateway] CARRIER_DISCORD_PUBLIC_KEY is not set — all Discord webhook requests will be rejected (401)");
  }
  const gatewayApiToken = loadGatewayAPIToken();
  if (!gatewayApiToken && !isLoopbackHost(hostname)) {
    throw new Error("CARRIER_GATEWAY_API_TOKEN is required when binding gateway to non-loopback host");
  }
  return Bun.serve({
    port,
    hostname,
    fetch: runtime.fetch,
  });
}

async function handleDownloadRequest(
  ctx: GatewayRequestContext,
  url: URL,
  readFile: ReadFileFn,
): Promise<Response> {
  const parsedPath = parseDownloadPath(url.pathname);
  if (!parsedPath) {
    return jsonResponse({
      requestId: ctx.requestId,
      result: "error",
      errorCode: "E_USAGE",
      message: "invalid download path",
    }, 400);
  }
  const { token, requestedFileName } = parsedPath;

  const resolved = ctx.deps.downloads.consume(token);
  if (!resolved) {
    return jsonResponse({
      requestId: ctx.requestId,
      result: "error",
      errorCode: "E_DOWNLOAD_TOKEN_INVALID",
      message: "download token is invalid or expired",
    }, 404);
  }

  const compareResult = compareRequestedFileName({
    requestedFileName,
    fileRef: resolved.fileRef,
  });
  if (!compareResult.matches) {
    return jsonResponse({
      requestId: ctx.requestId,
      result: "error",
      errorCode: "E_DOWNLOAD_FILE_MISMATCH",
      message: "requested filename does not match token artifact",
    }, 400);
  }

  const blob = await readFile(resolved.fileRef);
  if (!blob) {
    return jsonResponse({
      requestId: ctx.requestId,
      result: "error",
      errorCode: "E_DOWNLOAD_NOT_FOUND",
      message: "artifact file was not found",
    }, 404);
  }

  const headers = new Headers();
  headers.set("content-type", blob.type || "application/octet-stream");
  headers.set("content-disposition", buildContentDisposition(compareResult.expectedFileName));

  if (resolved.singleUse) {
    ctx.deps.downloads.finalizeConsumed(resolved.token);
  }

  return new Response(blob, {
    status: 200,
    headers,
  });
}

async function handleDiscordWebhookRequest(ctx: GatewayRequestContext): Promise<Response> {
  const body = await ctx.request.text();
  const signatureHex = ctx.request.headers.get("x-signature-ed25519");
  const timestamp = ctx.request.headers.get("x-signature-timestamp");
  const publicKeyHex = loadDiscordPublicKey();
  const verified = verifyDiscordRequestSignature({
    body,
    signatureHex,
    timestamp,
    publicKeyHex,
  });
  if (!verified) {
    return jsonResponse({
      requestId: ctx.requestId,
      result: "error",
      errorCode: "E_DISCORD_SIGNATURE_INVALID",
      message: "discord request signature verification failed",
    }, 401);
  }

  const payload = parseJSONText(body);
  if (!payload) {
    return jsonResponse({
      requestId: ctx.requestId,
      result: "error",
      errorCode: "E_USAGE",
      message: "request body must be valid JSON",
    }, 400);
  }

  const root = asRecord(payload);
  if (typeof root?.type === "number" && root.type === 1) {
    return jsonResponse({ type: 1 });
  }

  const normalized = parseDiscordPayloadToCommand(payload);
  if (!normalized) {
    return jsonResponse({
      requestId: ctx.requestId,
      result: "error",
      errorCode: "E_USAGE",
      message: "unsupported discord payload",
    }, 400);
  }

  const sessionToken = ctx.deps.sessions.getSession("discord", normalized.chatId)?.sessionToken ?? null;
  const commandInput = injectSessionTokenIfMissing(toGatewayInput(normalized), sessionToken);
  const response = await safeHandleCommand(commandInput, ctx.deps);
  const rendered = renderDiscordResponse(response);
  if (typeof root?.type === "number" && root.type === 2) {
    return jsonResponse({
      type: 4,
      data: rendered,
    });
  }

  return jsonResponse(rendered);
}

async function handleFeishuWebhookRequest(ctx: GatewayRequestContext): Promise<Response> {
  const payload = await parseJSONBody(ctx.request);
  if (!payload) {
    return jsonResponse({
      requestId: ctx.requestId,
      result: "error",
      errorCode: "E_USAGE",
      message: "request body must be valid JSON",
    }, 400);
  }

  const expectedToken = loadFeishuVerificationToken();
  if (!verifyFeishuEventToken(payload, expectedToken)) {
    return jsonResponse({
      requestId: ctx.requestId,
      result: "error",
      errorCode: "E_FEISHU_VERIFICATION_FAILED",
      message: "feishu event token verification failed",
    }, 401);
  }

  const challenge = extractFeishuURLVerificationChallenge(payload);
  if (challenge !== null) {
    return jsonResponse({ challenge });
  }

  const normalized = parseFeishuEventToCommand(payload);
  if (!normalized) {
    return jsonResponse({
      requestId: ctx.requestId,
      result: "ok",
      message: "ignored non-command feishu event",
    });
  }

  const sessionToken = ctx.deps.sessions.getSession("feishu", normalized.chatId)?.sessionToken ?? null;
  const commandInput = injectSessionTokenIfMissing(toGatewayInput(normalized), sessionToken);
  const response = await safeHandleCommand(commandInput, ctx.deps);
  return jsonResponse(renderFeishuResponse(response));
}

async function handleTelegramWebhookRequest(ctx: GatewayRequestContext): Promise<Response> {
  const payload = await parseJSONBody(ctx.request);
  if (!payload) {
    return jsonResponse({
      requestId: ctx.requestId,
      result: "error",
      errorCode: "E_USAGE",
      message: "request body must be valid JSON",
    }, 400);
  }

  const expectedSecret = loadTelegramWebhookSecret();
  const providedSecret = ctx.request.headers.get("x-telegram-bot-api-secret-token");
  if (!verifyTelegramWebhookSecret(providedSecret, expectedSecret)) {
    return jsonResponse({
      requestId: ctx.requestId,
      result: "error",
      errorCode: "E_TELEGRAM_VERIFICATION_FAILED",
      message: "telegram webhook secret verification failed",
    }, 401);
  }

  const normalized = parseTelegramUpdateToCommand(payload);
  if (!normalized) {
    return jsonResponse({
      requestId: ctx.requestId,
      result: "ok",
      message: "ignored non-command telegram update",
    });
  }

  const sessionToken = ctx.deps.sessions.getSession("telegram", normalized.chatId)?.sessionToken ?? null;
  const commandInput = injectSessionTokenIfMissing(toGatewayInput(normalized), sessionToken);
  const response = await safeHandleCommand(commandInput, ctx.deps);
  return jsonResponse(renderTelegramResponse(response));
}

function parseJSONText(raw: string): unknown | null {
  try {
    return JSON.parse(raw) as unknown;
  } catch {
    return null;
  }
}

async function parseJSONBody(request: Request): Promise<unknown | null> {
  try {
    return await request.json();
  } catch {
    return null;
  }
}

function extractFeishuURLVerificationChallenge(payload: unknown): string | null {
  const record = asRecord(payload);
  if (!record) {
    return null;
  }
  if (typeof record.type !== "string" || record.type !== "url_verification") {
    return null;
  }
  return typeof record.challenge === "string" ? record.challenge : "";
}

/**
 * Validates and resolves the artifact root directory to prevent security issues.
 * - Ensures the path is absolute
 * - Blocks dangerous paths like `/`, `/etc`, `/usr`, `/var`; `/root` exact only (subpaths allowed)
 * - Returns the resolved absolute path
 */
function validateAndResolveArtifactRoot(rootPath: string): string {
  const { resolve: pathResolve, sep } = require("node:path");
  
  // Resolve to absolute path
  const resolved = pathResolve(rootPath);
  
  // Block root directory
  if (resolved === sep) {
    throw new Error(`[security] ARTIFACT_ROOT cannot be system root: ${resolved}`);
  }
  
  // Block critical system directories
  // Block critical system directories (exact match + subdirectories).
  // /root is only blocked as exact match — scoped subdirectories like
  // /root/project/artifacts are legitimate in dev/CI environments.
  const blockedExactAndBelow = ["/etc", "/usr", "/var", "/bin", "/sbin", "/boot", "/sys", "/proc"];
  const blockedExactOnly = ["/root"];

  for (const dangerous of blockedExactAndBelow) {
    if (resolved === dangerous || resolved.startsWith(dangerous + sep)) {
      throw new Error(`[security] ARTIFACT_ROOT cannot be in system directory: ${resolved}`);
    }
  }
  for (const dangerous of blockedExactOnly) {
    if (resolved === dangerous) {
      throw new Error(`[security] ARTIFACT_ROOT cannot be in system directory: ${resolved}`);
    }
  }
  
  return resolved;
}

async function defaultReadFile(fileRef: string): Promise<Blob | null> {
  const { resolve: pathResolve, sep } = await import("node:path");
  const { realpath: fsRealpath } = await import("node:fs/promises");

  // Validate path to prevent traversal attacks - reject relative paths and .. components
  // Current callers are server-controlled, but this guards against future misuse
  if (!fileRef.startsWith("/") || fileRef.includes("/../") || fileRef.endsWith("/..")) {
    console.warn(`[security] rejected potentially unsafe fileRef: ${fileRef}`);
    return null;
  }

  // Enforce artifact-root boundary: the resolved canonical path must reside
  // under a configured root directory.  Defaults to ./artifacts subdirectory
  // instead of cwd to limit exposure.  This prevents reads of arbitrary absolute
  // paths (e.g. /etc/hosts) even when they are not symlinks.
  const artifactRoot = validateAndResolveArtifactRoot(
    process.env.ARTIFACT_ROOT ?? pathResolve(process.cwd(), "artifacts")
  );
  const canonicalRequested = pathResolve(fileRef);
  const rootPrefix = artifactRoot.endsWith(sep) ? artifactRoot : artifactRoot + sep;
  if (canonicalRequested !== artifactRoot && !canonicalRequested.startsWith(rootPrefix)) {
    console.warn(`[security] path outside artifact root: ${fileRef} resolves to ${canonicalRequested} (root: ${artifactRoot})`);
    return null;
  }

  const file = Bun.file(fileRef);
  if (!(await file.exists())) {
    return null;
  }

  // Resolve symlinks and verify the real path stays within the artifact root.
  // This prevents symlink traversal to arbitrary locations even when the
  // logical path looks clean.
  const resolvedPath = await fsRealpath(fileRef).catch(() => null);
  if (resolvedPath !== null && resolvedPath !== artifactRoot && !resolvedPath.startsWith(rootPrefix)) {
    console.warn(`[security] symlink traversal blocked: ${fileRef} resolves to ${resolvedPath} (root: ${artifactRoot})`);
    return null;
  }

  return file;
}

type ParsedCommandRequest = {
  commandInput: string | null;
  sessionToken: string | null;
};

async function parseCommandRequest(request: Request, maxBodyBytes: number = cachedMaxCommandBodyBytes): Promise<ParsedCommandRequest> {
  const allowAuthorizationSessionToken = !loadGatewayAPIToken();
  const rawBody = await readBodyWithLimit(request, maxBodyBytes);
  const result: ParsedCommandRequest = {
    commandInput: null,
    sessionToken: null,
  };

  // Prefer an explicit session token header when present.
  const sessionHeader = request.headers.get("x-session-token");
  if (sessionHeader && sessionHeader.trim().length > 0) {
    result.sessionToken = sessionHeader.trim();
  }

  // Backward-compatible session token transport: Authorization header.
  if (!result.sessionToken && allowAuthorizationSessionToken) {
    const authHeader = request.headers.get("authorization");
    if (authHeader) {
      const match = authHeader.match(/^Bearer\s+(.+)$/i);
      if (match && match[1]) {
        result.sessionToken = match[1].trim();
      }
    }
  }
  
  const contentType = request.headers.get("content-type")?.toLowerCase() || "";
  if (contentType.includes("application/json")) {
    try {
      const payload = JSON.parse(rawBody) as { input?: unknown; sessionToken?: unknown };
      
      // Extract command input
      if (typeof payload.input === "string" && payload.input.trim().length > 0) {
        result.commandInput = payload.input.trim();
      }
      
      // Extract session token from body (if not already set from header)
      if (!result.sessionToken && typeof payload.sessionToken === "string" && payload.sessionToken.trim().length > 0) {
        result.sessionToken = payload.sessionToken.trim();
      }
      
      return result;
    } catch {
      return result;
    }
  }

  // For non-JSON requests, treat the entire body as command input
  const raw = rawBody.trim();
  if (raw.length > 0) {
    result.commandInput = raw;
  }
  
  return result;
}

function loadMaxCommandBodyBytes(env: Record<string, string | undefined> = process.env): number {
  const raw = env.CARRIER_MAX_COMMAND_BODY_BYTES?.trim();
  if (!raw) {
    return DEFAULT_MAX_COMMAND_BODY_BYTES;
  }
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return DEFAULT_MAX_COMMAND_BODY_BYTES;
  }
  return parsed;
}

async function readBodyWithLimit(request: Request, maxBytes: number): Promise<string> {
  const contentLength = request.headers.get("content-length");
  if (contentLength) {
    const parsed = Number.parseInt(contentLength, 10);
    if (Number.isFinite(parsed) && parsed > maxBytes) {
      throw new PayloadTooLargeError(maxBytes);
    }
  }

  if (!request.body) {
    return "";
  }

  const reader = request.body.getReader();
  const decoder = new TextDecoder();
  let total = 0;
  let text = "";
  while (true) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }
    if (!value) {
      continue;
    }
    total += value.byteLength;
    if (total > maxBytes) {
      await reader.cancel();
      throw new PayloadTooLargeError(maxBytes);
    }
    text += decoder.decode(value, { stream: true });
  }
  text += decoder.decode();
  return text;
}

export function injectSessionTokenIfMissing(commandInput: string, sessionToken: string | null): string {
  if (!sessionToken) {
    return commandInput;
  }

  const parts = commandInput.trim().split(/\s+/);
  // provider chatId requestId /command ...
  if (parts.length < 4) {
    return commandInput;
  }

  const fourth = parts[3] ?? "";
  // If the 4th token is already not a command name, assume session token is inline.
  if (!fourth.startsWith("/")) {
    return commandInput;
  }

  const [provider, chatId, requestId, ...rest] = parts;
  return [provider, chatId, requestId, sessionToken, ...rest].join(" ");
}

function validateCommandAuth(
  commandInput: string,
  sessionToken: string | null,
  deps: GatewayDependencies,
): { requestId: string; result: "error"; errorCode: string; message: string } | null {
  // Parse the command to extract provider, chatId, requestId, and command name
  const parts = commandInput.trim().split(/\s+/);
  
  // For malformed commands (too few parts), skip auth validation and let the
  // command parser return the appropriate E_PARSE error
  if (parts.length < 4) {
    return null;
  }
  
  const [provider, chatId, requestId, commandName] = parts;
  
  // /pair command doesn't require authentication (it creates the session)
  if (commandName === "/pair") {
    return null;
  }
  
  // Check if session exists first for better error messages
  const session = deps.sessions.getSession(provider as any, chatId);
  if (!session) {
    // If no session exists, return E_SESSION_REQUIRED (backwards compatible)
    // The token requirement is implicit - can't have a token without a session
    return {
      requestId,
      result: "error",
      errorCode: "E_SESSION_REQUIRED",
      message: "chat is not paired; run /pair <code> first",
    };
  }
  
  // Session exists, now verify authentication token
  if (!sessionToken) {
    return {
      requestId,
      result: "error",
      errorCode: "E_AUTH_REQUIRED",
      message: "session token required (provide via Authorization header or sessionToken field)",
    };
  }
  
  if (session.sessionToken !== sessionToken) {
    return {
      requestId,
      result: "error",
      errorCode: "E_AUTH_INVALID",
      message: "invalid session token",
    };
  }
  
  // Authentication successful
  return null;
}

function loadGatewayAPIToken(): string | null {
  const token = process.env.CARRIER_GATEWAY_API_TOKEN?.trim() ?? "";
  return token.length > 0 ? token : null;
}

function loadDiscordPublicKey(): string | null {
  const key = process.env.CARRIER_DISCORD_PUBLIC_KEY?.trim() ?? "";
  return key.length > 0 ? key : null;
}

function loadFeishuVerificationToken(): string | null {
  const token = process.env.CARRIER_FEISHU_VERIFICATION_TOKEN?.trim() ?? "";
  return token.length > 0 ? token : null;
}

function loadTelegramWebhookSecret(): string | null {
  const token = process.env.CARRIER_TELEGRAM_WEBHOOK_SECRET?.trim() ?? "";
  return token.length > 0 ? token : null;
}

function isLoopbackHost(hostname: string): boolean {
  const normalized = hostname.trim().toLowerCase();
  return normalized === "127.0.0.1" || normalized === "::1" || normalized === "localhost";
}

function validateGatewayAPIToken(
  request: Request,
  requestId: string,
): { requestId: string; result: "error"; errorCode: string; message: string } | null {
  const expectedToken = loadGatewayAPIToken();
  if (!expectedToken) {
    return null;
  }

  const authHeader = request.headers.get("authorization");
  const match = authHeader?.match(/^Bearer\s+(.+)$/i);
  const providedToken = match?.[1]?.trim();
  if (!providedToken) {
    return {
      requestId,
      result: "error",
      errorCode: "E_GATEWAY_AUTH_REQUIRED",
      message: "gateway api token required",
    };
  }

  if (!constantTimeTokenEquals(providedToken, expectedToken)) {
    return {
      requestId,
      result: "error",
      errorCode: "E_GATEWAY_AUTH_INVALID",
      message: "invalid gateway api token",
    };
  }

  return null;
}

function constantTimeTokenEquals(left: string, right: string): boolean {
  const leftBuffer = Buffer.from(left, "utf8");
  const rightBuffer = Buffer.from(right, "utf8");
  if (leftBuffer.length !== rightBuffer.length) {
    return false;
  }
  return timingSafeEqual(leftBuffer, rightBuffer);
}

export function parsePort(raw: string | undefined, fallback: number): number {
  const value = (raw ?? "").trim();
  if (!/^[0-9]+$/.test(value)) {
    return fallback;
  }

  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < 1 || parsed > 65535) {
    return fallback;
  }
  return parsed;
}

function jsonResponse(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), {
    status,
    headers: {
      "content-type": "application/json",
    },
  });
}

if (import.meta.main) {
  const server = startGatewayServer();
  console.log(`gateway server listening on http://${server.hostname}:${server.port}`);

  // Graceful shutdown on SIGTERM and SIGINT
  const shutdown = () => {
    console.log("shutting down gateway server...");
    server.stop();
    process.exit(0);
  };

  process.on("SIGTERM", shutdown);
  process.on("SIGINT", shutdown);
}
