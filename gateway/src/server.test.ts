import { describe, expect, test } from "bun:test";
import { InMemoryDaemonClient } from "./daemon/client";
import { HttpDaemonClient } from "./daemon/http_client";
import { DownloadTokenStore } from "./downloads/token_store";
import { SessionStore } from "./session/store";
import {
  composeMiddleware,
  createGatewayRuntime,
  createRuntimeDependencies,
  requestIdMiddleware,
  type GatewayRequestContext,
} from "./server";

function makeDeps() {
  return {
    daemon: new InMemoryDaemonClient(),
    sessions: new SessionStore(),
    downloads: new DownloadTokenStore(),
  };
}

describe("composeMiddleware", () => {
  test("runs middleware in deterministic order", async () => {
    const trace: string[] = [];
    const deps = makeDeps();

    const middlewares = [
      async (_ctx: GatewayRequestContext, next: () => Promise<Response>) => {
        trace.push("mw1:before");
        const response = await next();
        trace.push("mw1:after");
        return response;
      },
      async (_ctx: GatewayRequestContext, next: () => Promise<Response>) => {
        trace.push("mw2:before");
        const response = await next();
        trace.push("mw2:after");
        return response;
      },
    ];

    const handler = composeMiddleware(middlewares, async () => {
      trace.push("handler");
      return new Response("ok");
    });

    await handler({
      request: new Request("http://gateway.local/healthz"),
      requestId: "",
      deps,
    });

    expect(trace).toEqual([
      "mw1:before",
      "mw2:before",
      "handler",
      "mw2:after",
      "mw1:after",
    ]);
  });
});

describe("runtime dependencies", () => {
  test("uses HTTP daemon client by default for runtime path", () => {
    const deps = createRuntimeDependencies();
    expect(deps.daemon).toBeInstanceOf(HttpDaemonClient);
    deps.downloads.stopPeriodicCleanup();
  });
});

describe("gateway runtime routes", () => {
  test("requestId middleware sets header", async () => {
    const deps = makeDeps();
    const runtime = createGatewayRuntime({
      deps,
      middlewares: [requestIdMiddleware],
    });

    const response = await runtime.fetch(new Request("http://gateway.local/healthz", {
      headers: {
        "x-request-id": "req-fixed",
      },
    }));

    expect(response.status).toBe(200);
    expect(response.headers.get("x-request-id")).toBe("req-fixed");
  });

  test("download route resolves valid token and enforces single-use", async () => {
    const deps = makeDeps();
    const filePath = `/tmp/gateway-download-${crypto.randomUUID()}.txt`;
    await Bun.write(filePath, "hello-download");
    const token = deps.downloads.issue(filePath, 300, true);
    const runtime = createGatewayRuntime({ deps });

    const first = await runtime.fetch(new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`));
    expect(first.status).toBe(200);
    expect(await first.text()).toBe("hello-download");

    const second = await runtime.fetch(new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`));
    expect(second.status).toBe(404);
  });

  test("download route rejects token filename mismatch", async () => {
    const deps = makeDeps();
    const filePath = `/tmp/gateway-download-${crypto.randomUUID()}.txt`;
    await Bun.write(filePath, "file-content");
    const token = deps.downloads.issue(filePath, 300, false);
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request(`http://gateway.local/downloads/${token.token}/wrong-name.txt`));
    expect(response.status).toBe(400);
    const payload = await response.json() as { errorCode?: string };
    expect(payload.errorCode).toBe("E_DOWNLOAD_FILE_MISMATCH");
  });

  test("download route escapes quotes in content-disposition filename", async () => {
    const deps = makeDeps();
    // Create a file with a name containing quotes and backslashes
    const fileName = 'test"file\\name.txt';
    const tmpDir = `/tmp/gateway-dl-test-${crypto.randomUUID()}`;
    const { mkdirSync } = await import("node:fs");
    mkdirSync(tmpDir, { recursive: true });
    const filePath = `${tmpDir}/${fileName}`;
    await Bun.write(filePath, "content");
    const token = deps.downloads.issue(filePath, 300, false);
    const runtime = createGatewayRuntime({ deps });

    // Manually construct URL with proper encoding
    const downloadUrl = `/downloads/${token.token}/${encodeURIComponent(fileName)}`;
    const response = await runtime.fetch(new Request(`http://gateway.local${downloadUrl}`));
    expect(response.status).toBe(200);
    
    const disposition = response.headers.get("content-disposition");
    // Should escape backslashes and quotes: test\"file\\\\name.txt
    expect(disposition).toBe('attachment; filename="test\\"file\\\\name.txt"');
  });

  test("command route executes command pipeline", async () => {
    const deps = makeDeps();
    deps.sessions.registerPairCode("pair-ok", 300);
    const runtime = createGatewayRuntime({ deps });

    const pairResponse = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ input: "telegram 100 req-1 /pair pair-ok" }),
    }));
    const pairPayload = await pairResponse.json() as { result: string };
    expect(pairPayload.result).toBe("ok");

    const agentsResponse = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ input: "telegram 100 req-2 /agents" }),
    }));
    const agentsPayload = await agentsResponse.json() as { result: string; message: string };
    expect(agentsPayload.result).toBe("ok");
    expect(agentsPayload.message).toContain("listed");
  });

  test("download route rejects path traversal in fileRef", async () => {
    const deps = makeDeps();
    // Create runtime with custom readFile that would expose traversal attempts
    let capturedPath: string | null = null;
    const runtime = createGatewayRuntime({
      deps,
      readFile: async (fileRef: string) => {
        capturedPath = fileRef;
        // Simulate the default behavior with path validation
        if (!fileRef.startsWith("/") || fileRef.includes("/../") || fileRef.endsWith("/..")) {
          return null;
        }
        return new Blob(["safe-content"]);
      },
    });

    // Try to create a token with a malicious path (in real scenario, this would be server-controlled)
    const maliciousPath = "/tmp/../etc/passwd";
    const token = deps.downloads.issue(maliciousPath, 300, false);

    const response = await runtime.fetch(new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`));
    
    // Should get 404 because readFile rejects the path
    expect(response.status).toBe(404);
    expect(capturedPath as string | null).toBe(maliciousPath);
  });
});
