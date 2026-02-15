import { safeHandleCommand, type GatewayDependencies } from "./index";
import { HttpDaemonClient } from "./daemon/http_client";
import { DownloadTokenStore } from "./downloads/token_store";
import { SessionStore } from "./session/store";

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
  const incoming = ctx.request.headers.get("x-request-id")?.trim();
  ctx.requestId = incoming && incoming.length > 0 ? incoming : crypto.randomUUID();
  const response = await next();
  response.headers.set("x-request-id", ctx.requestId);
  return response;
};

export function createRuntimeDependencies(overrides: Partial<GatewayDependencies> = {}): GatewayDependencies {
  return {
    daemon: overrides.daemon ?? new HttpDaemonClient(),
    sessions: overrides.sessions ?? new SessionStore(),
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
      const commandInput = await parseCommandInput(ctx.request);
      if (!commandInput) {
        return jsonResponse({
          requestId: ctx.requestId,
          result: "error",
          errorCode: "E_USAGE",
          message: "request body must provide command input",
        }, 400);
      }
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

async function defaultReadFile(fileRef: string): Promise<Blob | null> {
  // Validate path to prevent traversal attacks - reject relative paths and .. components
  // Current callers are server-controlled, but this guards against future misuse
  if (!fileRef.startsWith("/") || fileRef.includes("/../") || fileRef.endsWith("/..")) {
    console.warn(`[security] rejected potentially unsafe fileRef: ${fileRef}`);
    return null;
  }
  
  const file = Bun.file(fileRef);
  if (!(await file.exists())) {
    return null;
  }

  // Resolve symlinks to prevent symlink traversal attacks.
  // Reject any path where the resolved (real) path differs from the
  // original — this blocks symlinks pointing outside the intended directory.
  const { realpath } = await import("node:fs/promises");
  const resolvedPath = await realpath(fileRef).catch(() => null);
  if (resolvedPath !== null && resolvedPath !== fileRef) {
    console.warn(`[security] symlink traversal blocked: ${fileRef} resolves to ${resolvedPath}`);
    return null;
  }

  return file;
}

async function parseCommandInput(request: Request): Promise<string | null> {
  const contentType = request.headers.get("content-type")?.toLowerCase() || "";
  if (contentType.includes("application/json")) {
    try {
      const payload = await request.json() as { input?: unknown };
      if (typeof payload.input === "string" && payload.input.trim().length > 0) {
        return payload.input;
      }
      return null;
    } catch {
      return null;
    }
  }

  const raw = (await request.text()).trim();
  return raw.length > 0 ? raw : null;
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
}
