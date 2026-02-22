import { beforeEach, describe, expect, test } from "bun:test";
import { handleCommand, parseInput, ParseError, type GatewayDependencies } from "./index";
import { type DaemonAgentState, InMemoryDaemonClient } from "./daemon/client";
import { SessionStore } from "./session/store";
import { DownloadTokenStore } from "./downloads/token_store";

let deps: GatewayDependencies;

function buildDeps(): GatewayDependencies {
  return {
    daemon: new InMemoryDaemonClient(),
    sessions: new SessionStore(),
    downloads: new DownloadTokenStore(),
  };
}

function pairChat(d: GatewayDependencies, provider: "telegram" | "discord" | "feishu" = "telegram", chatId = "100"): void {
  d.sessions.registerPairCode("pair-ok", 300);
  d.sessions.pair({ provider, chatId, code: "pair-ok" });
}

function stubAgentList(d: GatewayDependencies, agents: DaemonAgentState[]): void {
  const daemon = d.daemon as InMemoryDaemonClient;
  (daemon as { listAgents: (ctx: unknown) => Promise<DaemonAgentState[]> }).listAgents = async () => agents;
}

describe("command routing: /pair", () => {
  beforeEach(() => {
    deps = buildDeps();
  });

  test("successful pairing returns session token", async () => {
    deps.sessions.registerPairCode("my-code", 300);

    const res = await handleCommand(parseInput("telegram 100 req-1 /pair my-code"), deps);

    expect(res.result).toBe("ok");
    expect(res.sessionToken).toBeString();
  });

  test("invalid code returns error", async () => {
    const res = await handleCommand(parseInput("telegram 100 req-1 /pair bad-code"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_PAIR_CODE_INVALID");
  });

  test("missing code returns usage error", async () => {
    const res = await handleCommand(parseInput("telegram 100 req-1 /pair"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });
});

describe("command routing: session requirement", () => {
  beforeEach(() => {
    deps = buildDeps();
  });

  test("unpaired chat gets E_SESSION_REQUIRED", async () => {
    const res = await handleCommand(parseInput("telegram 100 req-1 /agents"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_SESSION_REQUIRED");
  });
});

describe("command routing: /agents", () => {
  beforeEach(() => {
    deps = buildDeps();
    pairChat(deps);
  });

  test("lists agents for paired chat", async () => {
    const res = await handleCommand(parseInput("telegram 100 req-1 /agents"), deps);

    expect(res.result).toBe("ok");
    expect(res.message).toContain("listed");
  });

  test("returns error-first operational summary with lastError and guidance", async () => {
    const updatedAt = "2026-02-22T00:00:00.000Z";
    stubAgentList(deps, [
      {
        id: "agent-healthy",
        name: "Healthy Agent",
        version: "1.0.0",
        installed: true,
        runtimeState: "running",
        health: "healthy",
        needsRemoteDiagnosis: false,
        updatedAt,
      },
      {
        id: "agent-unhealthy",
        name: "Unhealthy Agent",
        version: "1.0.0",
        installed: true,
        runtimeState: "running",
        health: "unhealthy",
        needsRemoteDiagnosis: false,
        lastError: "panic: disk full",
        updatedAt,
      },
      {
        id: "agent-unknown",
        name: "Unknown Agent",
        version: "1.0.0",
        installed: false,
        runtimeState: "stopped",
        health: "unknown",
        needsRemoteDiagnosis: false,
        updatedAt,
      },
    ]);

    const res = await handleCommand(parseInput("telegram 100 req-1 /agents"), deps);

    expect(res.result).toBe("ok");
    expect(res.message).toContain("listed 3 agents (2 installed)");
    expect(res.message).toContain('agent-unhealthy: runtime=running health=unhealthy lastError="panic: disk full"');
    expect(res.message).toContain("drill-down: /status <agent_id> | /logs <agent_id> [tail] | /diagnose <agent_id>");

    const unhealthyPos = res.message.indexOf("agent-unhealthy: runtime=running health=unhealthy");
    const unknownPos = res.message.indexOf("agent-unknown: runtime=stopped health=unknown");
    const healthyPos = res.message.indexOf("agent-healthy: runtime=running health=healthy");
    expect(unhealthyPos).toBeGreaterThan(-1);
    expect(unknownPos).toBeGreaterThan(-1);
    expect(healthyPos).toBeGreaterThan(-1);
    expect(unhealthyPos).toBeLessThan(unknownPos);
    expect(unknownPos).toBeLessThan(healthyPos);
  });
});

describe("command routing: /install", () => {
  beforeEach(() => {
    deps = buildDeps();
    pairChat(deps);
  });

  test("missing agent returns usage error", async () => {
    const res = await handleCommand(parseInput("telegram 100 req-1 /install"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("installs agent successfully and updates state", async () => {
    const res = await handleCommand(parseInput("telegram 100 req-1 /install openclaw"), deps);

    expect(res.result).toBe("ok");
    expect(res.message).toContain("install completed");

    // Verify agent state was updated
    const statuses = await deps.daemon.getStatus("openclaw", { requestId: "verify", actor: "test" });
    expect(statuses).toHaveLength(1);
    expect(statuses[0].installed).toBe(true);
  });
});

describe("command routing: /start", () => {
  beforeEach(() => {
    deps = buildDeps();
    pairChat(deps);
  });

  test("start uninstalled agent returns error", async () => {
    const res = await handleCommand(parseInput("telegram 100 req-1 /start openclaw"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_NOT_INSTALLED");
  });

  test("start installed agent succeeds and agent becomes running", async () => {
    await handleCommand(parseInput("telegram 100 req-0 /install openclaw"), deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /start openclaw"), deps);

    expect(res.result).toBe("ok");

    // Verify agent state
    const statuses = await deps.daemon.getStatus("openclaw", { requestId: "verify", actor: "test" });
    expect(statuses[0].runtimeState).toBe("running");
    expect(statuses[0].health).toBe("healthy");
  });

  test("start already-running agent returns error", async () => {
    await handleCommand(parseInput("telegram 100 req-0 /install openclaw"), deps);
    await handleCommand(parseInput("telegram 100 req-0 /start openclaw"), deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /start openclaw"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_ALREADY_RUNNING");
  });
});

describe("command routing: /stop", () => {
  beforeEach(() => {
    deps = buildDeps();
    pairChat(deps);
  });

  test("stop already-stopped agent returns error", async () => {
    const res = await handleCommand(parseInput("telegram 100 req-1 /stop openclaw"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_ALREADY_STOPPED");
  });

  test("stop running agent succeeds and agent becomes stopped", async () => {
    await handleCommand(parseInput("telegram 100 req-0 /install openclaw"), deps);
    await handleCommand(parseInput("telegram 100 req-0 /start openclaw"), deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /stop openclaw"), deps);

    expect(res.result).toBe("ok");

    // Verify agent state
    const statuses = await deps.daemon.getStatus("openclaw", { requestId: "verify", actor: "test" });
    expect(statuses[0].runtimeState).toBe("stopped");
  });
});

describe("command routing: /status", () => {
  beforeEach(() => {
    deps = buildDeps();
    pairChat(deps);
  });

  test("returns status for paired chat", async () => {
    const res = await handleCommand(parseInput("telegram 100 req-1 /status openclaw"), deps);

    expect(res.result).toBe("ok");
    expect(res.message).toContain("status");
  });

  test("status with no agent id returns all", async () => {
    const res = await handleCommand(parseInput("telegram 100 req-1 /status"), deps);

    expect(res.result).toBe("ok");
  });
});

describe("command routing: /logs", () => {
  beforeEach(() => {
    deps = buildDeps();
    pairChat(deps);
  });

  test("missing agent returns usage error", async () => {
    const res = await handleCommand(parseInput("telegram 100 req-1 /logs"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("returns logs for agent", async () => {
    const res = await handleCommand(parseInput("telegram 100 req-1 /logs openclaw 10"), deps);

    expect(res.result).toBe("ok");
  });
});

describe("command routing: /upgrade", () => {
  beforeEach(() => {
    deps = buildDeps();
    pairChat(deps);
  });

  test("missing agent returns usage error", async () => {
    const res = await handleCommand(parseInput("telegram 100 req-1 /upgrade"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("upgrade succeeds", async () => {
    const res = await handleCommand(parseInput("telegram 100 req-1 /upgrade openclaw"), deps);

    expect(res.result).toBe("ok");
    expect(res.message).toContain("upgrade completed");
  });
});

describe("command routing: /diagnose", () => {
  beforeEach(() => {
    deps = buildDeps();
    pairChat(deps);
  });

  test("missing agent returns usage error", async () => {
    const res = await handleCommand(parseInput("telegram 100 req-1 /diagnose"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("diagnose returns download url", async () => {
    const res = await handleCommand(parseInput("telegram 100 req-1 /diagnose openclaw"), deps);

    expect(res.result).toBe("ok");
    expect(res.downloadUrl).toContain("/downloads/");
  });
});

describe("parseInput edge cases", () => {
  test("unknown command throws ParseError", () => {
    expect(() => parseInput("telegram 100 req-1 /unknown")).toThrow(ParseError);
  });

  test("too few parts throws ParseError", () => {
    expect(() => parseInput("telegram 100")).toThrow(ParseError);
  });
});
