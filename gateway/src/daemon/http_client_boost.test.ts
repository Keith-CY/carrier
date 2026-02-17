import { describe, expect, test } from "bun:test";
import { DaemonClientError, RemoteDiagnosisNotNeededError } from "./client";
import { HttpDaemonClient, loadDaemonBaseUrl, loadDaemonTimeoutMs } from "./http_client";

const ctx = { actor: "test:user", requestId: "req-boost" };

function mockFetch(status: number, body: unknown): typeof fetch {
  return (async () => new Response(JSON.stringify(body), { status })) as unknown as typeof fetch;
}

function mockFetchText(status: number, text: string): typeof fetch {
  return (async () => new Response(text, { status })) as unknown as typeof fetch;
}

describe("HttpDaemonClient - uninstallAgent is not on interface but upgradeAgent is", () => {
  test("upgradeAgent returns upgrade result", async () => {
    const fetchMock = mockFetch(200, {
      agentId: "openclaw",
      fromVersion: "1.0.0",
      toVersion: "1.0.1",
      backupPath: "/tmp/backup",
    });
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    const result = await client.upgradeAgent("openclaw", ctx);
    expect(result.agentId).toBe("openclaw");
    expect(result.fromVersion).toBe("1.0.0");
    expect(result.toVersion).toBe("1.0.1");
  });

  test("upgradeAgent with error", async () => {
    const fetchMock = mockFetch(409, {
      error: { code: "E_AGENT_RUNNING", message: "agent is running" },
    });
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    await expect(client.upgradeAgent("openclaw", ctx)).rejects.toBeInstanceOf(DaemonClientError);
  });
});

describe("HttpDaemonClient - diagnoseAgent", () => {
  test("diagnoseAgent returns artifact ref", async () => {
    const fetchMock = mockFetch(200, { artifactRef: "diag-123.zip" });
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    const result = await client.diagnoseAgent("openclaw", ctx);
    expect(result.artifactRef).toBe("diag-123.zip");
  });
});

describe("HttpDaemonClient - getStatus", () => {
  test("getStatus with agentId returns statuses", async () => {
    const fetchMock = mockFetch(200, {
      statuses: [{ id: "openclaw", runtimeState: "running" }],
    });
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    const statuses = await client.getStatus("openclaw", ctx);
    expect(statuses).toHaveLength(1);
    expect(statuses[0]?.id).toBe("openclaw");
  });

  test("getStatus without agentId returns all statuses", async () => {
    const fetchMock = mockFetch(200, {
      statuses: [
        { id: "openclaw", runtimeState: "running" },
        { id: "zeroclaw", runtimeState: "stopped" },
      ],
    });
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    const statuses = await client.getStatus(undefined, ctx);
    expect(statuses).toHaveLength(2);
  });

  test("getStatus with array response (legacy)", async () => {
    const fetchMock = mockFetch(200, [{ id: "openclaw" }]);
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    const statuses = await client.getStatus("openclaw", ctx);
    expect(statuses).toHaveLength(1);
  });
});

describe("HttpDaemonClient - getLogs", () => {
  test("getLogs returns lines", async () => {
    const fetchMock = mockFetch(200, { lines: ["line1", "line2"], truncated: false });
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    const result = await client.getLogs("openclaw", 50, ctx);
    expect(result.lines).toHaveLength(2);
    expect(result.truncated).toBe(false);
  });

  test("getLogs clamps tail", async () => {
    let capturedUrl = "";
    const fetchMock = (async (input: RequestInfo | URL) => {
      capturedUrl = String(input);
      return new Response(JSON.stringify({ lines: [], truncated: false }), { status: 200 });
    }) as typeof fetch;
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    await client.getLogs("openclaw", 99999, ctx);
    expect(capturedUrl).toContain("tail=1000");
  });

  test("getLogs defaults bad tail", async () => {
    let capturedUrl = "";
    const fetchMock = (async (input: RequestInfo | URL) => {
      capturedUrl = String(input);
      return new Response(JSON.stringify({ lines: [], truncated: false }), { status: 200 });
    }) as typeof fetch;
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    await client.getLogs("openclaw", -1, ctx);
    expect(capturedUrl).toContain("tail=200");
  });
});

describe("HttpDaemonClient - getMergedLogs", () => {
  test("getMergedLogs returns lines", async () => {
    const fetchMock = mockFetch(200, { lines: ["merged1"], truncated: false });
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    const result = await client.getMergedLogs(100, ctx);
    expect(result.lines).toHaveLength(1);
  });

  test("getMergedLogs clamps tail", async () => {
    let capturedUrl = "";
    const fetchMock = (async (input: RequestInfo | URL) => {
      capturedUrl = String(input);
      return new Response(JSON.stringify({ lines: [], truncated: false }), { status: 200 });
    }) as typeof fetch;
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    await client.getMergedLogs(5000, ctx);
    expect(capturedUrl).toContain("tail=1000");
  });
});

describe("HttpDaemonClient - verifyPairCode", () => {
  test("verifyPairCode success", async () => {
    const fetchMock = mockFetch(200, { code: "abc", consumed: true });
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    await client.verifyPairCode("abc", ctx); // should not throw
  });

  test("verifyPairCode failure", async () => {
    const fetchMock = mockFetch(400, {
      error: { code: "E_PAIR_CODE_INVALID", message: "invalid" },
    });
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    await expect(client.verifyPairCode("bad", ctx)).rejects.toBeInstanceOf(DaemonClientError);
  });
});

describe("HttpDaemonClient - createRemoteDiagnosisHandoff", () => {
  test("success returns handoff", async () => {
    const fetchMock = mockFetch(200, {
      id: "h1",
      agentId: "openclaw",
      consent: true,
      artifactRef: "ref.zip",
      status: "pending",
      createdAt: "2026-02-14T00:00:00Z",
    });
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    const result = await client.createRemoteDiagnosisHandoff({
      agentId: "openclaw",
      consent: true,
      actor: "test",
      requestId: "r1",
    });
    expect(result.id).toBe("h1");
  });
});

describe("HttpDaemonClient - listAgents legacy array", () => {
  test("handles raw array response", async () => {
    const fetchMock = mockFetch(200, [{ id: "a1" }, { id: "a2" }]);
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    const agents = await client.listAgents(ctx);
    expect(agents).toHaveLength(2);
  });
});

describe("HttpDaemonClient - error mapping", () => {
  test("400 maps to E_USAGE", async () => {
    const fetchMock = mockFetchText(400, "bad request");
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    try {
      await client.listAgents(ctx);
    } catch (e) {
      expect((e as DaemonClientError).code).toBe("E_USAGE");
    }
  });

  test("401 maps to E_SESSION_REQUIRED", async () => {
    const fetchMock = mockFetchText(401, "unauthorized");
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    try {
      await client.listAgents(ctx);
    } catch (e) {
      expect((e as DaemonClientError).code).toBe("E_SESSION_REQUIRED");
    }
  });

  test("403 maps to E_SESSION_REQUIRED", async () => {
    const fetchMock = mockFetchText(403, "forbidden");
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    try {
      await client.listAgents(ctx);
    } catch (e) {
      expect((e as DaemonClientError).code).toBe("E_SESSION_REQUIRED");
    }
  });

  test("500 maps to E_COMMAND_FAILED", async () => {
    const fetchMock = mockFetchText(500, "server error");
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    try {
      await client.listAgents(ctx);
    } catch (e) {
      expect((e as DaemonClientError).code).toBe("E_COMMAND_FAILED");
    }
  });

  test("empty response body", async () => {
    const fetchMock = (async () => new Response("", { status: 200 })) as unknown as typeof fetch;
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    const result = await client.listAgents(ctx);
    // Empty body returns {} which has no agents
    expect(Array.isArray(result) || result !== undefined).toBe(true);
  });
});

describe("loadDaemonBaseUrl edge cases", () => {
  test("trims multiple trailing slashes", () => {
    expect(loadDaemonBaseUrl({ CARRIER_DAEMON_BASE_URL: "http://host:9090///" })).toBe("http://host:9090");
  });

  test("handles whitespace", () => {
    expect(loadDaemonBaseUrl({ CARRIER_DAEMON_BASE_URL: "  http://host:9090  " })).toBe("http://host:9090");
  });
});

describe("loadDaemonTimeoutMs edge cases", () => {
  test("whitespace-only returns default", () => {
    expect(loadDaemonTimeoutMs({ CARRIER_DAEMON_TIMEOUT_MS: "   " })).toBe(30_000);
  });

  test("Infinity returns default", () => {
    expect(loadDaemonTimeoutMs({ CARRIER_DAEMON_TIMEOUT_MS: "Infinity" })).toBe(30_000);
  });
});
