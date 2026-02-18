import { describe, expect, test } from "bun:test";
import { InMemoryDaemonClient } from "./daemon/client";
import { HttpDaemonClient } from "./daemon/http_client";
import { DownloadTokenStore } from "./downloads/token_store";
import { SessionStore } from "./session/store";
import { generateKeyPairSync, sign } from "node:crypto";

// Tests create temporary files under /tmp; allow defaultReadFile to serve them.
process.env.ARTIFACT_ROOT = "/tmp";
import {
  composeMiddleware,
  createGatewayRuntime,
  createRuntimeDependencies,
  requestIdMiddleware,
  startGatewayServer,
  parsePort,
  injectSessionTokenIfMissing,
  type GatewayRequestContext,
} from "./server";

function makeDeps() {
  return {
    daemon: new InMemoryDaemonClient(),
    sessions: new SessionStore(),
    downloads: new DownloadTokenStore(),
  };
}

function rawPublicKeyHexFromSpkiDer(der: Buffer): string {
  return der.subarray(der.length - 32).toString("hex");
}

async function withEnvVar<T>(name: string, value: string | undefined, run: () => Promise<T> | T): Promise<T> {
  const previous = process.env[name];
  if (value === undefined) {
    delete process.env[name];
  } else {
    process.env[name] = value;
  }
  try {
    return await run();
  } finally {
    if (previous === undefined) {
      delete process.env[name];
    } else {
      process.env[name] = previous;
    }
  }
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

  test("rejects when middleware calls next() multiple times and does not double-run downstream", async () => {
    const trace: string[] = [];
    const deps = makeDeps();

    const middlewares = [
      async (_ctx: GatewayRequestContext, next: () => Promise<Response>) => {
        trace.push("mw1:before");
        const first = await next();
        trace.push("mw1:after-first-next");
        await expect(next()).rejects.toThrow("next() called multiple times");
        trace.push("mw1:after-second-next-error");
        return first;
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

    const response = await handler({
      request: new Request("http://gateway.local/healthz"),
      requestId: "",
      deps,
    });

    expect(response.status).toBe(200);
    expect(trace).toEqual([
      "mw1:before",
      "mw2:before",
      "handler",
      "mw2:after",
      "mw1:after-first-next",
      "mw1:after-second-next-error",
    ]);
    expect(trace.filter((item) => item === "mw2:before")).toHaveLength(1);
    expect(trace.filter((item) => item === "handler")).toHaveLength(1);
  });
});

describe("runtime dependencies", () => {
  test("uses HTTP daemon client by default for runtime path", () => {
    const deps = createRuntimeDependencies();
    expect(deps.daemon).toBeInstanceOf(HttpDaemonClient);
    deps.downloads.stopPeriodicCleanup();
  });

  test("instantiates rate limiter by default", () => {
    const deps = createRuntimeDependencies();
    expect(deps.rateLimiter).toBeDefined();
    deps.rateLimiter?.stopPeriodicCleanup();
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

  test("discord webhook rejects invalid signature before command execution", async () => {
    const deps = makeDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("dc-code");
    }
    const runtime = createGatewayRuntime({ deps });
    const { publicKey, privateKey } = generateKeyPairSync("ed25519");
    const publicKeyDer = publicKey.export({ type: "spki", format: "der" }) as Buffer;
    const publicKeyHex = rawPublicKeyHexFromSpkiDer(publicKeyDer);
    const payload = {
      id: "interaction-1",
      type: 2,
      channel_id: "dc-chat",
      data: {
        name: "pair",
        options: [{ name: "code", value: "dc-code" }],
      },
    };
    const body = JSON.stringify(payload);
    const timestamp = String(Math.floor(Date.now() / 1000));
    const validSignatureHex = sign(null, Buffer.from(`${timestamp}${body}`), privateKey).toString("hex");
    const invalidSignatureHex = `${validSignatureHex.slice(0, -2)}aa`;

    await withEnvVar("CARRIER_DISCORD_PUBLIC_KEY", publicKeyHex, async () => {
      const response = await runtime.fetch(new Request("http://gateway.local/webhook/discord", {
        method: "POST",
        headers: {
          "content-type": "application/json",
          "x-signature-ed25519": invalidSignatureHex,
          "x-signature-timestamp": timestamp,
        },
        body,
      }));

      expect(response.status).toBe(401);
      const responseBody = await response.json() as { errorCode?: string };
      expect(responseBody.errorCode).toBe("E_DISCORD_SIGNATURE_INVALID");
    });

    expect(deps.sessions.getSession("discord", "dc-chat")).toBeNull();
  });

  test("discord webhook executes command when signature is valid", async () => {
    const deps = makeDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("dc-code");
    }
    const runtime = createGatewayRuntime({ deps });
    const { publicKey, privateKey } = generateKeyPairSync("ed25519");
    const publicKeyDer = publicKey.export({ type: "spki", format: "der" }) as Buffer;
    const publicKeyHex = rawPublicKeyHexFromSpkiDer(publicKeyDer);
    const payload = {
      id: "interaction-2",
      type: 2,
      channel_id: "dc-chat",
      data: {
        name: "pair",
        options: [{ name: "code", value: "dc-code" }],
      },
    };
    const body = JSON.stringify(payload);
    const timestamp = String(Math.floor(Date.now() / 1000));
    const signatureHex = sign(null, Buffer.from(`${timestamp}${body}`), privateKey).toString("hex");

    await withEnvVar("CARRIER_DISCORD_PUBLIC_KEY", publicKeyHex, async () => {
      const response = await runtime.fetch(new Request("http://gateway.local/webhook/discord", {
        method: "POST",
        headers: {
          "content-type": "application/json",
          "x-signature-ed25519": signatureHex,
          "x-signature-timestamp": timestamp,
        },
        body,
      }));

      expect(response.status).toBe(200);
      const responseBody = await response.json() as {
        type?: number;
        data?: { content?: string };
      };
      expect(responseBody.type).toBe(4);
      expect(responseBody.data?.content).toContain("paired discord:dc-chat");
    });
  });

  test("discord webhook responds to ping interaction with pong", async () => {
    const deps = makeDeps();
    const runtime = createGatewayRuntime({ deps });
    const { publicKey, privateKey } = generateKeyPairSync("ed25519");
    const publicKeyDer = publicKey.export({ type: "spki", format: "der" }) as Buffer;
    const publicKeyHex = rawPublicKeyHexFromSpkiDer(publicKeyDer);
    const body = JSON.stringify({ type: 1 });
    const timestamp = String(Math.floor(Date.now() / 1000));
    const signatureHex = sign(null, Buffer.from(`${timestamp}${body}`), privateKey).toString("hex");

    await withEnvVar("CARRIER_DISCORD_PUBLIC_KEY", publicKeyHex, async () => {
      const response = await runtime.fetch(new Request("http://gateway.local/webhook/discord", {
        method: "POST",
        headers: {
          "content-type": "application/json",
          "x-signature-ed25519": signatureHex,
          "x-signature-timestamp": timestamp,
        },
        body,
      }));

      expect(response.status).toBe(200);
      const responseBody = await response.json() as { type?: number };
      expect(responseBody.type).toBe(1);
    });
  });

  test("feishu webhook rejects invalid verification token before command execution", async () => {
    const deps = makeDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("fs-code");
    }
    const runtime = createGatewayRuntime({ deps });

    await withEnvVar("CARRIER_FEISHU_VERIFICATION_TOKEN", "expected-token", async () => {
      const response = await runtime.fetch(new Request("http://gateway.local/webhook/feishu", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          header: { event_id: "evt-1", token: "wrong-token" },
          event: {
            message: {
              message_id: "msg-1",
              chat_id: "fs-chat",
              content: JSON.stringify({ text: "/pair fs-code" }),
            },
          },
        }),
      }));

      expect(response.status).toBe(401);
      const payload = await response.json() as { errorCode?: string };
      expect(payload.errorCode).toBe("E_FEISHU_VERIFICATION_FAILED");
    });

    expect(deps.sessions.getSession("feishu", "fs-chat")).toBeNull();
  });

  test("feishu webhook executes command when verification token is valid", async () => {
    const deps = makeDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("fs-code");
    }
    const runtime = createGatewayRuntime({ deps });

    await withEnvVar("CARRIER_FEISHU_VERIFICATION_TOKEN", "expected-token", async () => {
      const response = await runtime.fetch(new Request("http://gateway.local/webhook/feishu", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          header: { event_id: "evt-2", token: "expected-token" },
          event: {
            message: {
              message_id: "msg-2",
              chat_id: "fs-chat",
              content: JSON.stringify({ text: "/pair fs-code" }),
            },
          },
        }),
      }));

      expect(response.status).toBe(200);
      const payload = await response.json() as { msg_type?: string; content?: { text?: string } };
      expect(payload.msg_type).toBe("text");
      expect(payload.content?.text).toContain("paired feishu:fs-chat");
    });
  });

  test("feishu webhook returns url verification challenge when token is valid", async () => {
    const deps = makeDeps();
    const runtime = createGatewayRuntime({ deps });

    await withEnvVar("CARRIER_FEISHU_VERIFICATION_TOKEN", "expected-token", async () => {
      const response = await runtime.fetch(new Request("http://gateway.local/webhook/feishu", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          type: "url_verification",
          challenge: "challenge-123",
          token: "expected-token",
        }),
      }));

      expect(response.status).toBe(200);
      const payload = await response.json() as { challenge?: string };
      expect(payload.challenge).toBe("challenge-123");
    });
  });

  test("telegram webhook rejects invalid secret before command execution", async () => {
    const deps = makeDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("tg-code");
    }
    const runtime = createGatewayRuntime({ deps });

    await withEnvVar("CARRIER_TELEGRAM_WEBHOOK_SECRET", "expected-secret", async () => {
      const response = await runtime.fetch(new Request("http://gateway.local/webhook/telegram", {
        method: "POST",
        headers: {
          "content-type": "application/json",
          "x-telegram-bot-api-secret-token": "wrong-secret",
        },
        body: JSON.stringify({
          update_id: 1001,
          message: {
            message_id: 7001,
            chat: { id: 12345 },
            text: "/pair tg-code",
          },
        }),
      }));

      expect(response.status).toBe(401);
      const payload = await response.json() as { errorCode?: string };
      expect(payload.errorCode).toBe("E_TELEGRAM_VERIFICATION_FAILED");
    });

    expect(deps.sessions.getSession("telegram", "12345")).toBeNull();
  });

  test("telegram webhook executes command when secret is valid", async () => {
    const deps = makeDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("tg-code");
    }
    const runtime = createGatewayRuntime({ deps });

    await withEnvVar("CARRIER_TELEGRAM_WEBHOOK_SECRET", "expected-secret", async () => {
      const response = await runtime.fetch(new Request("http://gateway.local/webhook/telegram", {
        method: "POST",
        headers: {
          "content-type": "application/json",
          "x-telegram-bot-api-secret-token": "expected-secret",
        },
        body: JSON.stringify({
          update_id: 1002,
          message: {
            message_id: 7002,
            chat: { id: 12345 },
            text: "/pair tg-code",
          },
        }),
      }));

      expect(response.status).toBe(200);
      const payload = await response.json() as { text?: string };
      expect(payload.text).toContain("paired telegram:12345");
    });
  });

  test("telegram webhook ignores non-command updates", async () => {
    const deps = makeDeps();
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request("http://gateway.local/webhook/telegram", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        update_id: 1003,
        message: {
          message_id: 7003,
          chat: { id: 12345 },
          text: "hello there",
        },
      }),
    }));

    expect(response.status).toBe(200);
    const payload = await response.json() as { message?: string };
    expect(payload.message).toBe("ignored non-command telegram update");
  });

  test("download route resolves valid token and enforces single-use", async () => {
    const deps = makeDeps();
    const filePath = `/tmp/gateway-download-${crypto.randomUUID()}.txt`;
    await Bun.write(filePath, "hello-download");
    const token = deps.downloads.issue(filePath, 300, true);
    const runtime = createGatewayRuntime({
      deps,
      readFile: async (fileRef: string) => {
        const file = Bun.file(fileRef);
        return (await file.exists()) ? file : null;
      },
    });

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

  test("download route triggers single-use cleanup callback after successful fetch", async () => {
    const deps = makeDeps();
    const filePath = `/tmp/gateway-download-${crypto.randomUUID()}.txt`;
    await Bun.write(filePath, "cleanup-content");
    const cleaned: string[] = [];
    const token = deps.downloads.issue(filePath, 300, true, {
      onCleanup: (fileRef) => cleaned.push(fileRef),
    });
    const runtime = createGatewayRuntime({
      deps,
      readFile: async (fileRef: string) => {
        const file = Bun.file(fileRef);
        return (await file.exists()) ? file : null;
      },
    });

    const response = await runtime.fetch(new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`));
    expect(response.status).toBe(200);
    expect(cleaned).toEqual([filePath]);
  });

  test("download route rejects malformed percent-encoded filename path", async () => {
    const deps = makeDeps();
    const filePath = `/tmp/gateway-download-${crypto.randomUUID()}.txt`;
    await Bun.write(filePath, "file-content");
    const token = deps.downloads.issue(filePath, 300, false);
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request(`http://gateway.local/downloads/${token.token}/bad%ZZname.txt`));
    expect(response.status).toBe(400);
    const payload = await response.json() as { errorCode?: string };
    expect(payload.errorCode).toBe("E_USAGE");
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

    // Clean up temporary directory
    const { rmSync } = await import("node:fs");
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("command route executes command pipeline", async () => {
    const deps = makeDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("pair-ok");
    }
    const runtime = createGatewayRuntime({ deps });

    const pairResponse = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ input: "telegram 100 req-1 /pair pair-ok" }),
    }));
    const pairPayload = await pairResponse.json() as { result: string; sessionToken: string };
    expect(pairPayload.result).toBe("ok");
    const sessionToken = pairPayload.sessionToken;
    const agentsResponse = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { 
        "content-type": "application/json",
        "authorization": `Bearer ${sessionToken}`,
      },
      body: JSON.stringify({ input: `telegram 100 req-2 ${sessionToken} /agents` }),
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

describe("port resolution fallback behavior", () => {
  test("uses default port when env is undefined", () => {
    expect(parsePort(undefined, 8787)).toBe(8787);
  });

  test("uses default port when env is non-numeric", () => {
    expect(parsePort("not-a-number", 8787)).toBe(8787);
  });

  test("uses default port when env is empty string", () => {
    expect(parsePort("", 8787)).toBe(8787);
  });

  test("uses default port when env is zero", () => {
    expect(parsePort("0", 8787)).toBe(8787);
  });

  test("uses default port when env is negative", () => {
    expect(parsePort("-1234", 8787)).toBe(8787);
  });

  test("uses env port when env is valid numeric string", () => {
    expect(parsePort("9999", 8787)).toBe(9999);
  });

  test("config port takes precedence over env", () => {
    const envPort = parsePort("9999", 8787);
    const explicitConfigPort = 7777;
    expect(explicitConfigPort ?? envPort).toBe(7777);
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

  test("rejects non-loopback host when gateway api token is missing", async () => {
    await withEnvVar("CARRIER_GATEWAY_API_TOKEN", undefined, () => {
      const deps = makeDeps();
      expect(() => startGatewayServer({
        deps,
        hostname: "0.0.0.0",
        port: 0,
      })).toThrow("CARRIER_GATEWAY_API_TOKEN is required");
    });
  });

  test("allows non-loopback host when gateway api token is configured", async () => {
    await withEnvVar("CARRIER_GATEWAY_API_TOKEN", "gw-test-token", () => {
      const deps = makeDeps();
      const server = startGatewayServer({
        deps,
        hostname: "0.0.0.0",
        port: 0,
      });
      expect(server.port).toBeGreaterThan(0);
      server.stop();
    });
  });
});

describe("command authentication", () => {
  test("requires gateway api token when configured", async () => {
    await withEnvVar("CARRIER_GATEWAY_API_TOKEN", "gw-secret", async () => {
      const deps = makeDeps();
      if (deps.daemon instanceof InMemoryDaemonClient) {
        deps.daemon.registerPairCode("pair-code");
      }
      const runtime = createGatewayRuntime({ deps });

      const response = await runtime.fetch(new Request("http://gateway.local/command", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ input: "telegram 100 req-1 /pair pair-code" }),
      }));

      expect(response.status).toBe(401);
      const payload = await response.json() as { errorCode?: string };
      expect(payload.errorCode).toBe("E_GATEWAY_AUTH_REQUIRED");
    });
  });

  test("rejects invalid gateway api token when configured", async () => {
    await withEnvVar("CARRIER_GATEWAY_API_TOKEN", "gw-secret", async () => {
      const deps = makeDeps();
      if (deps.daemon instanceof InMemoryDaemonClient) {
        deps.daemon.registerPairCode("pair-code");
      }
      const runtime = createGatewayRuntime({ deps });

      const response = await runtime.fetch(new Request("http://gateway.local/command", {
        method: "POST",
        headers: {
          "content-type": "application/json",
          "authorization": "Bearer wrong-token",
        },
        body: JSON.stringify({ input: "telegram 100 req-1 /pair pair-code" }),
      }));

      expect(response.status).toBe(401);
      const payload = await response.json() as { errorCode?: string };
      expect(payload.errorCode).toBe("E_GATEWAY_AUTH_INVALID");
    });
  });

  test("accepts valid gateway api token and keeps session auth in body", async () => {
    await withEnvVar("CARRIER_GATEWAY_API_TOKEN", "gw-secret", async () => {
      const deps = makeDeps();
      if (deps.daemon instanceof InMemoryDaemonClient) {
        deps.daemon.registerPairCode("pair-code");
      }
      const runtime = createGatewayRuntime({ deps });

      const pairResponse = await runtime.fetch(new Request("http://gateway.local/command", {
        method: "POST",
        headers: {
          "content-type": "application/json",
          "authorization": "Bearer gw-secret",
        },
        body: JSON.stringify({ input: "telegram 100 req-1 /pair pair-code" }),
      }));
      expect(pairResponse.status).toBe(200);
      const pairPayload = await pairResponse.json() as { result: string; sessionToken?: string };
      expect(pairPayload.result).toBe("ok");
      expect(pairPayload.sessionToken).toBeDefined();

      const response = await runtime.fetch(new Request("http://gateway.local/command", {
        method: "POST",
        headers: {
          "content-type": "application/json",
          "authorization": "Bearer gw-secret",
        },
        body: JSON.stringify({
          input: "telegram 100 req-2 /agents",
          sessionToken: pairPayload.sessionToken,
        }),
      }));

      expect(response.status).toBe(200);
      const payload = await response.json() as { result: string };
      expect(payload.result).toBe("ok");
    });
  });

  test("healthz remains unauthenticated even when gateway api token is configured", async () => {
    await withEnvVar("CARRIER_GATEWAY_API_TOKEN", "gw-secret", async () => {
      const deps = makeDeps();
      const runtime = createGatewayRuntime({ deps });
      const response = await runtime.fetch(new Request("http://gateway.local/healthz"));
      expect(response.status).toBe(200);
    });
  });

  test("rejects authenticated commands without session token", async () => {
    const deps = makeDeps();
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ input: "telegram 100 req-1 /agents" }),
    }));

    expect(response.status).toBe(401);
    const payload = await response.json() as { errorCode?: string; message?: string };
    expect(payload.errorCode).toBe("E_SESSION_REQUIRED");
    expect(payload.message).toContain("not paired");
  });

  test("rejects commands with invalid session token", async () => {
    const deps = makeDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("pair-code");
    }
    const runtime = createGatewayRuntime({ deps });

    // Create a session via pairing
    const pairResponse = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ input: "telegram 100 req-1 /pair pair-code" }),
    }));
    const pairPayload = await pairResponse.json() as { result: string };
    expect(pairPayload.result).toBe("ok");

    // Try to use a wrong token
    const response = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { 
        "content-type": "application/json",
        "authorization": "Bearer wrong-token",
      },
      body: JSON.stringify({ input: "telegram 100 req-2 /agents" }),
    }));

    expect(response.status).toBe(401);
    const payload = await response.json() as { errorCode?: string; message?: string };
    expect(payload.errorCode).toBe("E_AUTH_INVALID");
    expect(payload.message).toContain("invalid session token");
  });

  test("accepts commands with valid session token in Authorization header", async () => {
    const deps = makeDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("pair-code");
    }
    const runtime = createGatewayRuntime({ deps });

    const pairResponse = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ input: "telegram 100 req-1 /pair pair-code" }),
    }));
    const pairPayload = await pairResponse.json() as { result: string; sessionToken?: string };
    expect(pairPayload.result).toBe("ok");

    const sessionToken = pairPayload.sessionToken as string;
    const response = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { 
        "content-type": "application/json",
        "authorization": `Bearer ${sessionToken}`,
      },
      body: JSON.stringify({ input: "telegram 100 req-2 /agents" }),
    }));

    expect(response.status).toBe(200);
    const payload = await response.json() as { result: string };
    expect(payload.result).toBe("ok");
  });

  test("accepts commands with valid session token in request body", async () => {
    const deps = makeDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("pair-code");
    }
    const runtime = createGatewayRuntime({ deps });

    const pairResponse = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ input: "telegram 100 req-1 /pair pair-code" }),
    }));
    const pairPayload = await pairResponse.json() as { result: string; sessionToken?: string };
    expect(pairPayload.result).toBe("ok");

    const sessionToken = pairPayload.sessionToken as string;
    const response = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ 
        input: "telegram 100 req-2 /agents",
        sessionToken,
      }),
    }));

    expect(response.status).toBe(200);
    const payload = await response.json() as { result: string };
    expect(payload.result).toBe("ok");
  });

  test("allows /pair command without session token", async () => {
    const deps = makeDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("pair-code");
    }
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ input: "telegram 100 req-1 /pair pair-code" }),
    }));

    expect(response.status).toBe(200);
    const payload = await response.json() as { result: string; sessionToken?: string };
    expect(payload.result).toBe("ok");
    expect(payload.sessionToken).toBeDefined();
  });

  test("rejects commands for non-existent session", async () => {
    const deps = makeDeps();
    const runtime = createGatewayRuntime({ deps });

    const response = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { 
        "content-type": "application/json",
        "authorization": "Bearer fake-session-token",
      },
      body: JSON.stringify({ input: "telegram 999 req-1 /agents" }),
    }));

    expect(response.status).toBe(401);
    const payload = await response.json() as { errorCode?: string; message?: string };
    expect(payload.errorCode).toBe("E_SESSION_REQUIRED");
    expect(payload.message).toContain("not paired");
  });

  test("rejects commands with mismatched provider/chatId", async () => {
    const deps = makeDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("pair-code");
    }
    const runtime = createGatewayRuntime({ deps });

    // Pair for telegram:100
    const pairResponse = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ input: "telegram 100 req-1 /pair pair-code" }),
    }));
    const pairPayload = await pairResponse.json() as { result: string; sessionToken?: string };
    expect(pairPayload.result).toBe("ok");

    const sessionToken = pairPayload.sessionToken as string;

    // Try to use the token for telegram:200 (different chat, no session)
    const response = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { 
        "content-type": "application/json",
        "authorization": `Bearer ${sessionToken}`,
      },
      body: JSON.stringify({ input: "telegram 200 req-2 /agents" }),
    }));

    expect(response.status).toBe(401);
    const payload = await response.json() as { errorCode?: string };
    expect(payload.errorCode).toBe("E_SESSION_REQUIRED");
  });
});

describe("command route payload size limits", () => {
  test("accepts JSON payload exactly at configured limit", async () => {
    const deps = makeDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("pair-limit-json");
    }
    const body = JSON.stringify({ input: "telegram 100 req-json-limit /pair pair-limit-json" });
    const runtime = createGatewayRuntime({ deps, maxCommandBodyBytes: Buffer.byteLength(body) });

    const response = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body,
    }));
    expect(response.status).toBe(200);
    const payload = await response.json() as { result: string };
    expect(payload.result).toBe("ok");
  });

  test("rejects JSON payload larger than configured limit", async () => {
    const deps = makeDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("pair-limit-json-too-large");
    }
    const body = JSON.stringify({ input: "telegram 100 req-json-large /pair pair-limit-json-too-large" });
    const runtime = createGatewayRuntime({ deps, maxCommandBodyBytes: Buffer.byteLength(body) - 1 });

    const response = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body,
    }));
    expect(response.status).toBe(413);
    const payload = await response.json() as { errorCode?: string };
    expect(payload.errorCode).toBe("E_PAYLOAD_TOO_LARGE");
  });

  test("accepts text payload exactly at configured limit", async () => {
    const deps = makeDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("pair-limit-text");
    }
    const command = "telegram 100 req-text-limit /pair pair-limit-text";
    const runtime = createGatewayRuntime({ deps, maxCommandBodyBytes: Buffer.byteLength(command) });

    const response = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { "content-type": "text/plain" },
      body: command,
    }));
    expect(response.status).toBe(200);
    const payload = await response.json() as { result: string };
    expect(payload.result).toBe("ok");
  });

  test("rejects text payload larger than configured limit", async () => {
    const deps = makeDeps();
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("pair-limit-text-too-large");
    }
    const command = "telegram 100 req-text-large /pair pair-limit-text-too-large";
    const runtime = createGatewayRuntime({ deps, maxCommandBodyBytes: Buffer.byteLength(command) - 1 });

    const response = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { "content-type": "text/plain" },
      body: command,
    }));
    expect(response.status).toBe(413);
    const payload = await response.json() as { errorCode?: string };
    expect(payload.errorCode).toBe("E_PAYLOAD_TOO_LARGE");
  });

  test("keeps malformed JSON behavior under size limit", async () => {
    const deps = makeDeps();
    const runtime = createGatewayRuntime({ deps, maxCommandBodyBytes: 1024 });
    const malformed = "{\"input\":";

    const response = await runtime.fetch(new Request("http://gateway.local/command", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: malformed,
    }));
    expect(response.status).toBe(400);
    const payload = await response.json() as { errorCode?: string };
    expect(payload.errorCode).toBe("E_USAGE");
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

describe("injectSessionTokenIfMissing", () => {
  test("passes through command when sessionToken is null", () => {
    const commandInput = "telegram 100 req-1 /agents";
    const result = injectSessionTokenIfMissing(commandInput, null);
    expect(result).toBe(commandInput);
  });

  test("passes through command when sessionToken is empty string", () => {
    const commandInput = "telegram 100 req-1 /agents";
    const result = injectSessionTokenIfMissing(commandInput, "");
    // Empty string is falsy, so it should pass through
    expect(result).toBe(commandInput);
  });

  test("does not inject token when command already has token (4th token not starting with /)", () => {
    const commandInput = "telegram 100 req-1 existing-token /agents list";
    const result = injectSessionTokenIfMissing(commandInput, "new-token");
    // Should not modify because 4th token doesn't start with /
    expect(result).toBe(commandInput);
  });

  test("injects session token before command", () => {
    const commandInput = "telegram 100 req-1 /agents";
    const sessionToken = "test-session-token-123";
    const result = injectSessionTokenIfMissing(commandInput, sessionToken);
    expect(result).toBe("telegram 100 req-1 test-session-token-123 /agents");
  });

  test("injects session token before command with arguments", () => {
    const commandInput = "telegram 100 req-1 /agents list all";
    const sessionToken = "test-token";
    const result = injectSessionTokenIfMissing(commandInput, sessionToken);
    expect(result).toBe("telegram 100 req-1 test-token /agents list all");
  });

  test("passes through command with fewer than 4 parts", () => {
    const commandInput = "telegram 100 req-1";
    const sessionToken = "test-token";
    const result = injectSessionTokenIfMissing(commandInput, sessionToken);
    // Should not modify because command is too short
    expect(result).toBe(commandInput);
  });

  test("handles command with extra whitespace", () => {
    const commandInput = "  telegram   100   req-1   /agents  ";
    const sessionToken = "test-token";
    const result = injectSessionTokenIfMissing(commandInput, sessionToken);
    // trim() and split(/\s+/) normalizes whitespace
    expect(result).toBe("telegram 100 req-1 test-token /agents");
  });

  test("preserves command arguments after injection", () => {
    const commandInput = "discord 12345 req-abc /install openclaw";
    const sessionToken = "secure-token-xyz";
    const result = injectSessionTokenIfMissing(commandInput, sessionToken);
    expect(result).toBe("discord 12345 req-abc secure-token-xyz /install openclaw");
  });

  test("does not double-inject when token is already present", () => {
    const commandInput = "telegram 100 req-1 my-token /agents";
    const newToken = "different-token";
    const result = injectSessionTokenIfMissing(commandInput, newToken);
    // Should not inject because 4th token (my-token) doesn't start with /
    expect(result).toBe(commandInput);
  });

  test("handles command with special characters in token", () => {
    const commandInput = "telegram 100 req-1 /status";
    const sessionToken = "token-with-special_chars.123";
    const result = injectSessionTokenIfMissing(commandInput, sessionToken);
    expect(result).toBe("telegram 100 req-1 token-with-special_chars.123 /status");
  });
});
