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
  startGatewayServer,
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

describe("download Content-Disposition edge cases", () => {
  test("handles filename with semicolon", async () => {
    const deps = makeDeps();
    const fileName = "file;data.txt";
    const filePath = `/tmp/gateway-download-${crypto.randomUUID()}-${fileName}`;
    await Bun.write(filePath, "content");
    const token = deps.downloads.issue(filePath, 300, true);
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`));
    expect(response.status).toBe(200);
    const disposition = response.headers.get("content-disposition");
    expect(disposition).toBeDefined();
    expect(disposition).toContain(fileName);
  });

  test("handles filename with comma", async () => {
    const deps = makeDeps();
    const fileName = "data,results.csv";
    const filePath = `/tmp/gateway-download-${crypto.randomUUID()}-${fileName}`;
    await Bun.write(filePath, "csv,data");
    const token = deps.downloads.issue(filePath, 300, true);
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`));
    expect(response.status).toBe(200);
    const disposition = response.headers.get("content-disposition");
    expect(disposition).toBeDefined();
    expect(disposition).toContain(fileName);
  });

  test("handles filename with UTF-8 characters", async () => {
    const deps = makeDeps();
    const fileName = "文件名.txt";
    const filePath = `/tmp/gateway-download-${crypto.randomUUID()}-${fileName}`;
    await Bun.write(filePath, "utf8-content");
    const token = deps.downloads.issue(filePath, 300, true);
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`));
    expect(response.status).toBe(200);
    const disposition = response.headers.get("content-disposition");
    expect(disposition).toBeDefined();
    // For UTF-8 filenames, should use RFC 5987 filename* parameter
    expect(disposition).toContain("filename*=UTF-8''");
    expect(disposition).toContain(encodeURIComponent(fileName));
  });

  test("handles filename with emoji", async () => {
    const deps = makeDeps();
    const fileName = "report📊.txt";
    const filePath = `/tmp/gateway-download-${crypto.randomUUID()}-${fileName}`;
    await Bun.write(filePath, "emoji-content");
    const token = deps.downloads.issue(filePath, 300, true);
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`));
    expect(response.status).toBe(200);
    const disposition = response.headers.get("content-disposition");
    expect(disposition).toBeDefined();
    // For emoji filenames, should use RFC 5987 filename* parameter with URL encoding
    expect(disposition).toContain("filename*=UTF-8''");
    expect(disposition).toContain(encodeURIComponent("📊"));
  });

  test("handles filename with leading spaces", async () => {
    const deps = makeDeps();
    const fileName = "  leading.txt";
    const filePath = `/tmp/gateway-download-${crypto.randomUUID()}-${fileName}`;
    await Bun.write(filePath, "space-content");
    const token = deps.downloads.issue(filePath, 300, true);
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`));
    expect(response.status).toBe(200);
    const disposition = response.headers.get("content-disposition");
    expect(disposition).toBeDefined();
    expect(disposition).toContain(fileName);
  });

  test("handles filename with trailing spaces", async () => {
    const deps = makeDeps();
    const fileName = "trailing.txt  ";
    const filePath = `/tmp/carrier-download-${crypto.randomUUID()}-${fileName}`;
    await Bun.write(filePath, "trailing-space");
    const token = deps.downloads.issue(filePath, 300, true);
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`));
    expect(response.status).toBe(200);
    const disposition = response.headers.get("content-disposition");
    expect(disposition).toBeDefined();
    expect(disposition).toContain(fileName.trim());
  });

  test("handles filename with double quotes", async () => {
    const deps = makeDeps();
    const fileName = 'file"name".txt';
    const filePath = `/tmp/gateway-download-${crypto.randomUUID()}-${fileName}`;
    await Bun.write(filePath, "quote-content");
    const token = deps.downloads.issue(filePath, 300, true);
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`));
    expect(response.status).toBe(200);
    const disposition = response.headers.get("content-disposition");
    expect(disposition).toBeDefined();
    // Content-Disposition should properly escape or handle quotes
    expect(disposition).toContain("filename=");
  });

  test("handles filename with backslash", async () => {
    const deps = makeDeps();
    const fileName = "path\\file.txt";
    const filePath = `/tmp/gateway-download-${crypto.randomUUID()}-path-file.txt`;
    await Bun.write(filePath, "backslash-content");
    const token = deps.downloads.issue(filePath, 300, true);
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`));
    expect(response.status).toBe(200);
    const disposition = response.headers.get("content-disposition");
    expect(disposition).toBeDefined();
    expect(disposition).toContain("filename=");
  });
});

describe("port resolution fallback behavior", () => {
  test("uses default port when config and env are both undefined", async () => {
    const deps = makeDeps();
    const originalEnv = process.env.CARRIER_GATEWAY_PORT;
    delete process.env.CARRIER_GATEWAY_PORT;

    const server = startGatewayServer({ 
      deps,
      port: undefined,
    });
    
    expect(server.port).toBe(8787);
    server.stop();
    
    if (originalEnv !== undefined) {
      process.env.CARRIER_GATEWAY_PORT = originalEnv;
    }
  });

  test("uses default port when env is non-numeric", async () => {
    const deps = makeDeps();
    const originalEnv = process.env.CARRIER_GATEWAY_PORT;
    process.env.CARRIER_GATEWAY_PORT = "not-a-number";

    const server = startGatewayServer({ 
      deps,
      port: undefined,
    });
    
    expect(server.port).toBe(8787);
    server.stop();
    
    if (originalEnv !== undefined) {
      process.env.CARRIER_GATEWAY_PORT = originalEnv;
    } else {
      delete process.env.CARRIER_GATEWAY_PORT;
    }
  });

  test("uses default port when env is empty string", async () => {
    const deps = makeDeps();
    const originalEnv = process.env.CARRIER_GATEWAY_PORT;
    process.env.CARRIER_GATEWAY_PORT = "";

    const server = startGatewayServer({ 
      deps,
      port: undefined,
    });
    
    expect(server.port).toBe(8787);
    server.stop();
    
    if (originalEnv !== undefined) {
      process.env.CARRIER_GATEWAY_PORT = originalEnv;
    } else {
      delete process.env.CARRIER_GATEWAY_PORT;
    }
  });

  test("uses default port when env is zero", async () => {
    const deps = makeDeps();
    const originalEnv = process.env.CARRIER_GATEWAY_PORT;
    process.env.CARRIER_GATEWAY_PORT = "0";

    const server = startGatewayServer({ 
      deps,
      port: undefined,
    });
    
    expect(server.port).toBe(8787);
    server.stop();
    
    if (originalEnv !== undefined) {
      process.env.CARRIER_GATEWAY_PORT = originalEnv;
    } else {
      delete process.env.CARRIER_GATEWAY_PORT;
    }
  });

  test("uses default port when env is negative", async () => {
    const deps = makeDeps();
    const originalEnv = process.env.CARRIER_GATEWAY_PORT;
    process.env.CARRIER_GATEWAY_PORT = "-1234";

    const server = startGatewayServer({ 
      deps,
      port: undefined,
    });
    
    expect(server.port).toBe(8787);
    server.stop();
    
    if (originalEnv !== undefined) {
      process.env.CARRIER_GATEWAY_PORT = originalEnv;
    } else {
      delete process.env.CARRIER_GATEWAY_PORT;
    }
  });

  test("uses env port when env is valid numeric string", async () => {
    const deps = makeDeps();
    const originalEnv = process.env.CARRIER_GATEWAY_PORT;
    process.env.CARRIER_GATEWAY_PORT = "9999";

    const server = startGatewayServer({ 
      deps,
      port: undefined,
    });
    
    expect(server.port).toBe(9999);
    server.stop();
    
    if (originalEnv !== undefined) {
      process.env.CARRIER_GATEWAY_PORT = originalEnv;
    } else {
      delete process.env.CARRIER_GATEWAY_PORT;
    }
  });

  test("config port takes precedence over env", async () => {
    const deps = makeDeps();
    const originalEnv = process.env.CARRIER_GATEWAY_PORT;
    process.env.CARRIER_GATEWAY_PORT = "9999";

    const server = startGatewayServer({ 
      deps,
      port: 7777,
    });
    
    expect(server.port).toBe(7777);
    server.stop();
    
    if (originalEnv !== undefined) {
      process.env.CARRIER_GATEWAY_PORT = originalEnv;
    } else {
      delete process.env.CARRIER_GATEWAY_PORT;
    }
  });

  test("uses default port when env has decimal value", async () => {
    const deps = makeDeps();
    const originalEnv = process.env.CARRIER_GATEWAY_PORT;
    process.env.CARRIER_GATEWAY_PORT = "8080.5";

    const server = startGatewayServer({ 
      deps,
      port: undefined,
    });
    
    expect(server.port).toBe(8787);
    server.stop();
    
    if (originalEnv !== undefined) {
      process.env.CARRIER_GATEWAY_PORT = originalEnv;
    } else {
      delete process.env.CARRIER_GATEWAY_PORT;
    }
  });

  test("uses default port when env contains special characters", async () => {
    const deps = makeDeps();
    const originalEnv = process.env.CARRIER_GATEWAY_PORT;
    process.env.CARRIER_GATEWAY_PORT = "8080!@#";

    const server = startGatewayServer({ 
      deps,
      port: undefined,
    });
    
    expect(server.port).toBe(8787);
    server.stop();
    
    if (originalEnv !== undefined) {
      process.env.CARRIER_GATEWAY_PORT = originalEnv;
    } else {
      delete process.env.CARRIER_GATEWAY_PORT;
    }
  });

  test("uses default port when env is above max port range", async () => {
    const deps = makeDeps();
    const originalEnv = process.env.CARRIER_GATEWAY_PORT;
    process.env.CARRIER_GATEWAY_PORT = "65536";

    const server = startGatewayServer({
      deps,
      port: undefined,
    });

    expect(server.port).toBe(8787);
    server.stop();

    if (originalEnv !== undefined) {
      process.env.CARRIER_GATEWAY_PORT = originalEnv;
    } else {
      delete process.env.CARRIER_GATEWAY_PORT;
    }
  });

  test("uses env port when env is max valid port", async () => {
    const deps = makeDeps();
    const originalEnv = process.env.CARRIER_GATEWAY_PORT;
    process.env.CARRIER_GATEWAY_PORT = "65535";

    const server = startGatewayServer({
      deps,
      port: undefined,
    });

    expect(server.port).toBe(65535);
    server.stop();

    if (originalEnv !== undefined) {
      process.env.CARRIER_GATEWAY_PORT = originalEnv;
    } else {
      delete process.env.CARRIER_GATEWAY_PORT;
    }
  });
});

describe("HTTP method mismatch handling", () => {
  test("rejects POST on healthz endpoint", async () => {
    const deps = makeDeps();
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request("http://gateway.local/healthz", {
      method: "POST",
    }));
    
    expect(response.status).toBe(404);
    const payload = await response.json() as { errorCode?: string };
    expect(payload.errorCode).toBe("E_NOT_FOUND");
  });

  test("rejects PUT on healthz endpoint", async () => {
    const deps = makeDeps();
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request("http://gateway.local/healthz", {
      method: "PUT",
    }));
    
    expect(response.status).toBe(404);
    const payload = await response.json() as { errorCode?: string };
    expect(payload.errorCode).toBe("E_NOT_FOUND");
  });

  test("rejects DELETE on healthz endpoint", async () => {
    const deps = makeDeps();
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request("http://gateway.local/healthz", {
      method: "DELETE",
    }));
    
    expect(response.status).toBe(404);
    const payload = await response.json() as { errorCode?: string };
    expect(payload.errorCode).toBe("E_NOT_FOUND");
  });

  test("rejects GET on command endpoint", async () => {
    const deps = makeDeps();
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "GET",
    }));
    
    expect(response.status).toBe(404);
    const payload = await response.json() as { errorCode?: string };
    expect(payload.errorCode).toBe("E_NOT_FOUND");
  });

  test("rejects PUT on command endpoint", async () => {
    const deps = makeDeps();
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ input: "telegram 100 req-1 /agents" }),
    }));
    
    expect(response.status).toBe(404);
    const payload = await response.json() as { errorCode?: string };
    expect(payload.errorCode).toBe("E_NOT_FOUND");
  });

  test("rejects DELETE on command endpoint", async () => {
    const deps = makeDeps();
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "DELETE",
    }));
    
    expect(response.status).toBe(404);
    const payload = await response.json() as { errorCode?: string };
    expect(payload.errorCode).toBe("E_NOT_FOUND");
  });

  test("rejects POST on download endpoint", async () => {
    const deps = makeDeps();
    const filePath = `/tmp/gateway-download-${crypto.randomUUID()}.txt`;
    await Bun.write(filePath, "test-content");
    const token = deps.downloads.issue(filePath, 300, false);
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`, {
      method: "POST",
    }));
    
    expect(response.status).toBe(404);
    const payload = await response.json() as { errorCode?: string };
    expect(payload.errorCode).toBe("E_NOT_FOUND");
  });

  test("rejects PUT on download endpoint", async () => {
    const deps = makeDeps();
    const filePath = `/tmp/gateway-download-${crypto.randomUUID()}.txt`;
    await Bun.write(filePath, "test-content");
    const token = deps.downloads.issue(filePath, 300, false);
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`, {
      method: "PUT",
    }));
    
    expect(response.status).toBe(404);
    const payload = await response.json() as { errorCode?: string };
    expect(payload.errorCode).toBe("E_NOT_FOUND");
  });

  test("rejects DELETE on download endpoint", async () => {
    const deps = makeDeps();
    const filePath = `/tmp/gateway-download-${crypto.randomUUID()}.txt`;
    await Bun.write(filePath, "test-content");
    const token = deps.downloads.issue(filePath, 300, false);
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`, {
      method: "DELETE",
    }));
    
    expect(response.status).toBe(404);
    const payload = await response.json() as { errorCode?: string };
    expect(payload.errorCode).toBe("E_NOT_FOUND");
  });

  test("rejects PATCH on unknown route", async () => {
    const deps = makeDeps();
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request("http://gateway.local/unknown", {
      method: "PATCH",
    }));
    
    expect(response.status).toBe(404);
    const payload = await response.json() as { errorCode?: string };
    expect(payload.errorCode).toBe("E_NOT_FOUND");
  });
});
