import { safeHandleCommand, type GatewayDependencies } from "./index";
import { HttpDaemonClient } from "./daemon/http_client";
import { DownloadTokenStore } from "./downloads/token_store";
import { SessionStore } from "./session/store";
import { join } from "node:path";

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
};

export type GatewayRuntime = {
  deps: GatewayDependencies;
  fetch: (request: Request) => Promise<Response>;
};

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
    rateLimiter: overrides.rateLimiter,
  };
}

export function createGatewayRuntime(options: GatewayRuntimeOptions = {}): GatewayRuntime {
  const deps = createRuntimeDependencies(options.deps);
  const readFile = options.readFile ?? defaultReadFile;
  const middlewares = options.middlewares ?? [requestIdMiddleware];

  const router: GatewayHandler = async (ctx) => {
    const url = new URL(ctx.request.url);
    if (ctx.request.method === "GET" && url.pathname === "/healthz") {
      return jsonResponse({
        status: "ok",
      });
    }

    if (ctx.request.method === "POST" && url.pathname === "/command") {
      const parsed = await parseCommandRequest(ctx.request);
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
  const parts = url.pathname.split("/").filter(Boolean);
  if (parts.length !== 3 || parts[0] !== "downloads") {
    return jsonResponse({
      requestId: ctx.requestId,
      result: "error",
      errorCode: "E_USAGE",
      message: "invalid download path",
    }, 400);
  }

  const token = parts[1] ?? "";
  let requestedFileName: string;
  try {
    requestedFileName = decodeURIComponent(parts[2] ?? "");
  } catch {
    return jsonResponse({
      requestId: ctx.requestId,
      result: "error",
      errorCode: "E_USAGE",
      message: "invalid download path",
    }, 400);
  }

  const resolved = ctx.deps.downloads.consume(token);
  if (!resolved) {
    return jsonResponse({
      requestId: ctx.requestId,
      result: "error",
      errorCode: "E_DOWNLOAD_TOKEN_INVALID",
      message: "download token is invalid or expired",
    }, 404);
  }

  const expectedFileName = resolved.fileRef.split("/").pop() || "artifact.bin";
  const normalizedExpected = expectedFileName.trim();
  const normalizedRequested = requestedFileName.trim();
  
  if (normalizedRequested !== normalizedExpected) {
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
  headers.set("content-disposition", buildContentDisposition(normalizedExpected));

  if (resolved.singleUse) {
    ctx.deps.downloads.finalizeConsumed(resolved.token);
  }

  return new Response(blob, {
    status: 200,
    headers,
  });
}

function buildContentDisposition(filename: string): string {
  // Check if filename contains non-ASCII characters
  const hasNonASCII = /[^\x00-\x7F]/.test(filename);
  
  if (hasNonASCII) {
    // For non-ASCII filenames, use an ASCII-safe fallback in filename parameter
    // and the full UTF-8 filename in filename* parameter (RFC 5987)
    const asciiFallback = filename.replace(/[^\x00-\x7F]/g, "_");
    const escapedFallback = asciiFallback.replace(/\\/g, "\\\\").replace(/"/g, '\\"');
    const encodedFilename = encodeURIComponent(filename);
    return `attachment; filename="${escapedFallback}"; filename*=UTF-8''${encodedFilename}`;
  }
  
  // For ASCII filenames, just escape quotes and backslashes
  const escapedFilename = filename.replace(/\\/g, "\\\\").replace(/"/g, '\\"');
  return `attachment; filename="${escapedFilename}"`;
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

async function parseCommandRequest(request: Request): Promise<ParsedCommandRequest> {
  const result: ParsedCommandRequest = {
    commandInput: null,
    sessionToken: null,
  };
  
  // Check Authorization header for session token
  const authHeader = request.headers.get("authorization");
  if (authHeader) {
    const match = authHeader.match(/^Bearer\s+(.+)$/i);
    if (match && match[1]) {
      result.sessionToken = match[1].trim();
    }
  }
  
  const contentType = request.headers.get("content-type")?.toLowerCase() || "";
  if (contentType.includes("application/json")) {
    try {
      const payload = await request.json() as { input?: unknown; sessionToken?: unknown };
      
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
  const raw = (await request.text()).trim();
  if (raw.length > 0) {
    result.commandInput = raw;
  }
  
  return result;
}

function injectSessionTokenIfMissing(commandInput: string, sessionToken: string | null): string {
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
