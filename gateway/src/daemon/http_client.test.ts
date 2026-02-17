import { describe, expect, test } from "bun:test";
import { DaemonClientError, RemoteDiagnosisNotNeededError } from "./client";
import { HttpDaemonClient, loadDaemonBaseUrl } from "./http_client";

function makeFetch(response: Response): typeof fetch {
  return (async () => response.clone()) as unknown as typeof fetch;
}

describe("loadDaemonBaseUrl", () => {
  test("uses default when env is missing", () => {
    expect(loadDaemonBaseUrl({})).toBe("http://127.0.0.1:9090");
  });

  test("trims trailing slash from env value", () => {
    expect(loadDaemonBaseUrl({ CARRIER_DAEMON_BASE_URL: "http://localhost:8080/" })).toBe("http://localhost:8080");
  });
});

describe("HttpDaemonClient", () => {
  test("listAgents parses envelope payload", async () => {
    const fetchMock = makeFetch(new Response(JSON.stringify({
      agents: [
        {
          id: "openclaw",
          name: "OpenClaw",
          version: "1.0.0",
          installed: true,
          runtimeState: "running",
          health: "healthy",
          needsRemoteDiagnosis: false,
          updatedAt: "2026-02-14T16:00:00.000Z",
        },
      ],
    }), { status: 200 }));
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);

    const agents = await client.listAgents({ actor: "telegram:100", requestId: "req-1" });
    expect(agents).toHaveLength(1);
    expect(agents[0]?.id).toBe("openclaw");
    expect(agents[0]?.runtimeState).toBe("running");
  });

  test("maps daemon error envelope to DaemonClientError", async () => {
    const fetchMock = makeFetch(new Response(JSON.stringify({
      error: {
        code: "E_ALREADY_STOPPED",
        message: "agent is already stopped",
      },
    }), { status: 409 }));
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);

    await expect(client.stopAgent("openclaw", { actor: "telegram:100", requestId: "req-2" }))
      .rejects.toBeInstanceOf(DaemonClientError);

    try {
      await client.stopAgent("openclaw", { actor: "telegram:100", requestId: "req-2" });
    } catch (error) {
      const daemonError = error as DaemonClientError;
      expect(daemonError.code).toBe("E_ALREADY_STOPPED");
      expect(daemonError.message).toContain("already stopped");
    }
  });

  test("maps status-only error to fallback code", async () => {
    const fetchMock = makeFetch(new Response("not found", { status: 404 }));
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);

    try {
      await client.listAgents({ actor: "telegram:100", requestId: "req-3" });
    } catch (error) {
      const daemonError = error as DaemonClientError;
      expect(daemonError.code).toBe("E_AGENT_NOT_FOUND");
    }
  });

  test("maps E_REMOTE_DIAG_NOT_NEEDED to specialized error", async () => {
    const fetchMock = makeFetch(new Response(JSON.stringify({
      error: {
        code: "E_REMOTE_DIAG_NOT_NEEDED",
        message: "remote diagnosis is not required for this agent",
      },
    }), { status: 400 }));
    const client = new HttpDaemonClient("http://daemon.local", fetchMock);

    await expect(client.createRemoteDiagnosisHandoff({
      agentId: "openclaw",
      consent: true,
      actor: "telegram:100",
      requestId: "req-4",
    })).rejects.toBeInstanceOf(RemoteDiagnosisNotNeededError);
  });
});

describe("HttpDaemonClient header propagation", () => {
  test("listAgents propagates actor and request-id headers", async () => {
    let capturedHeaders: HeadersInit | undefined;
    const fetchMock = (async (input: RequestInfo | URL, init?: RequestInit) => {
      capturedHeaders = init?.headers;
      return new Response(JSON.stringify({ agents: [] }), { status: 200 });
    }) as typeof fetch;

    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    await client.listAgents({ actor: "discord:channel:12345", requestId: "req-list-1" });

    expect(capturedHeaders).toBeDefined();
    const headers = new Headers(capturedHeaders as HeadersInit);
    expect(headers.get("x-carrier-actor")).toBe("discord:channel:12345");
    expect(headers.get("x-carrier-request-id")).toBe("req-list-1");
  });

  test("startAgent propagates actor and request-id headers", async () => {
    let capturedHeaders: HeadersInit | undefined;
    const fetchMock = (async (input: RequestInfo | URL, init?: RequestInit) => {
      capturedHeaders = init?.headers;
      return new Response(JSON.stringify({}), { status: 200 });
    }) as typeof fetch;

    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    await client.startAgent("openclaw", { actor: "telegram:user:999", requestId: "req-start-1" });

    expect(capturedHeaders).toBeDefined();
    const headers = new Headers(capturedHeaders as HeadersInit);
    expect(headers.get("x-carrier-actor")).toBe("telegram:user:999");
    expect(headers.get("x-carrier-request-id")).toBe("req-start-1");
  });

  test("stopAgent propagates actor and request-id headers", async () => {
    let capturedHeaders: HeadersInit | undefined;
    const fetchMock = (async (input: RequestInfo | URL, init?: RequestInit) => {
      capturedHeaders = init?.headers;
      return new Response(JSON.stringify({}), { status: 200 });
    }) as typeof fetch;

    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    await client.stopAgent("openclaw", { actor: "whatsapp:chat:777", requestId: "req-stop-1" });

    expect(capturedHeaders).toBeDefined();
    const headers = new Headers(capturedHeaders as HeadersInit);
    expect(headers.get("x-carrier-actor")).toBe("whatsapp:chat:777");
    expect(headers.get("x-carrier-request-id")).toBe("req-stop-1");
  });

  test("installAgent propagates actor and request-id headers", async () => {
    let capturedHeaders: HeadersInit | undefined;
    const fetchMock = (async (input: RequestInfo | URL, init?: RequestInit) => {
      capturedHeaders = init?.headers;
      return new Response(JSON.stringify({}), { status: 200 });
    }) as typeof fetch;

    const client = new HttpDaemonClient("http://daemon.local", fetchMock);
    await client.installAgent("newagent", { actor: "cli:user:alice", requestId: "req-install-1" });

    expect(capturedHeaders).toBeDefined();
    const headers = new Headers(capturedHeaders as HeadersInit);
    expect(headers.get("x-carrier-actor")).toBe("cli:user:alice");
    expect(headers.get("x-carrier-request-id")).toBe("req-install-1");
  });
});
