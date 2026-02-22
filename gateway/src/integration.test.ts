import { describe, expect, test } from "bun:test";
import { safeHandleCommand, type GatewayDependencies } from "./index";
import { type DaemonAgentState, InMemoryDaemonClient } from "./daemon/client";
import { SessionStore } from "./session/store";
import { DownloadTokenStore } from "./downloads/token_store";

function buildDeps(): GatewayDependencies {
  return {
    daemon: new InMemoryDaemonClient(),
    sessions: new SessionStore(),
    downloads: new DownloadTokenStore(),
  };
}

function pair(deps: GatewayDependencies, provider = "telegram", chatId = "100"): void {
  deps.sessions.registerPairCode("test-code", 300);
  deps.sessions.pair({ provider: provider as "telegram", chatId, code: "test-code" });
}

function stubAgentList(deps: GatewayDependencies, agents: DaemonAgentState[]): void {
  const daemon = deps.daemon as InMemoryDaemonClient;
  (daemon as { listAgents: (ctx: unknown) => Promise<DaemonAgentState[]> }).listAgents = async () => agents;
}

describe("E2E: pair → install → start → status → stop flow", () => {
  test("full lifecycle succeeds", async () => {
    const deps = buildDeps();

    // 1. Pair
    deps.sessions.registerPairCode("my-code", 300);
    const pairRes = await safeHandleCommand("telegram 100 req-pair /pair my-code", deps);
    expect(pairRes.result).toBe("ok");
    expect(pairRes.sessionToken).toBeDefined();

    // 2. Install
    const installRes = await safeHandleCommand("telegram 100 req-install /install openclaw", deps);
    expect(installRes.result).toBe("ok");
    expect(installRes.message).toContain("install completed");

    // 3. Start
    const startRes = await safeHandleCommand("telegram 100 req-start /start openclaw", deps);
    expect(startRes.result).toBe("ok");
    expect(startRes.message).toContain("start completed");

    // 4. Status — should show running/healthy
    const statusRes = await safeHandleCommand("telegram 100 req-status /status openclaw", deps);
    expect(statusRes.result).toBe("ok");
    expect(statusRes.message).toContain("running");
    expect(statusRes.message).toContain("healthy");

    // 5. Stop
    const stopRes = await safeHandleCommand("telegram 100 req-stop /stop openclaw", deps);
    expect(stopRes.result).toBe("ok");
    expect(stopRes.message).toContain("stop completed");

    // 6. Status after stop — should show stopped
    const statusAfterStop = await safeHandleCommand("telegram 100 req-status2 /status openclaw", deps);
    expect(statusAfterStop.result).toBe("ok");
    expect(statusAfterStop.message).toContain("stopped");
  });

  test("commands fail without pairing", async () => {
    const deps = buildDeps();
    const res = await safeHandleCommand("telegram 100 req-1 /install openclaw", deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_SESSION_REQUIRED");
  });

  test("start fails before install", async () => {
    const deps = buildDeps();
    pair(deps);
    const res = await safeHandleCommand("telegram 100 req-1 /start openclaw", deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_NOT_INSTALLED");
  });

  test("double start fails", async () => {
    const deps = buildDeps();
    pair(deps);
    await safeHandleCommand("telegram 100 req-1 /install openclaw", deps);
    await safeHandleCommand("telegram 100 req-2 /start openclaw", deps);
    const res = await safeHandleCommand("telegram 100 req-3 /start openclaw", deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_ALREADY_RUNNING");
  });

  test("stop when already stopped fails", async () => {
    const deps = buildDeps();
    pair(deps);
    await safeHandleCommand("telegram 100 req-1 /install openclaw", deps);
    const res = await safeHandleCommand("telegram 100 req-2 /stop openclaw", deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_ALREADY_STOPPED");
  });
});

describe("E2E: diagnose flow with download token", () => {
  test("diagnose generates artifact and download URL", async () => {
    const deps = buildDeps();
    pair(deps);
    await safeHandleCommand("telegram 100 req-1 /install openclaw", deps);

    const res = await safeHandleCommand("telegram 100 req-diag /diagnose openclaw", deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("diagnose artifact prepared");
    expect(res.downloadUrl).toBeDefined();
    expect(res.downloadUrl).toContain("/downloads/");
  });

  test("diagnose-consent flow with remote diagnosis needed", async () => {
    const deps = buildDeps();
    pair(deps);

    const daemon = deps.daemon as InMemoryDaemonClient;
    daemon.setDiagnoseArtifact("openclaw", "/tmp/openclaw-diag.zip");
    daemon.setRemoteDiagnosisState("openclaw", true);

    const res = await safeHandleCommand("telegram 100 req-consent /diagnose-consent openclaw yes", deps);
    expect(res.result).toBe("ok");
    expect(res.handoffId).toBeDefined();
    expect(res.handoffStatus).toBe("pending");
    expect(res.downloadUrl).toContain("/downloads/");
  });

  test("diagnose-consent declined returns declined status", async () => {
    const deps = buildDeps();
    pair(deps);

    const daemon = deps.daemon as InMemoryDaemonClient;
    daemon.setRemoteDiagnosisState("openclaw", true);

    const res = await safeHandleCommand("telegram 100 req-consent /diagnose-consent openclaw no", deps);
    expect(res.result).toBe("ok");
    expect(res.handoffStatus).toBe("declined");
  });

  test("diagnose-consent fails when not needed", async () => {
    const deps = buildDeps();
    pair(deps);

    const res = await safeHandleCommand("telegram 100 req-1 /diagnose-consent openclaw yes", deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_REMOTE_DIAG_NOT_NEEDED");
  });
});

describe("E2E: error propagation", () => {
  test("unknown agent returns E_AGENT_NOT_FOUND", async () => {
    const deps = buildDeps();
    pair(deps);
    const res = await safeHandleCommand("telegram 100 req-1 /install nonexistent", deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_AGENT_NOT_FOUND");
  });

  test("invalid command returns parse error", async () => {
    const deps = buildDeps();
    const res = await safeHandleCommand("bad input", deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_PARSE");
  });

  test("unknown command name returns parse error", async () => {
    const deps = buildDeps();
    const res = await safeHandleCommand("telegram 100 req-1 /unknown", deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_PARSE");
  });

  test("missing args return usage error", async () => {
    const deps = buildDeps();
    pair(deps);

    const commands = ["/install", "/start", "/stop", "/logs", "/upgrade", "/diagnose"];
    for (const cmd of commands) {
      const res = await safeHandleCommand(`telegram 100 req-1 ${cmd}`, deps);
      expect(res.result).toBe("error");
      expect(res.errorCode).toBe("E_USAGE");
    }
  });

  test("error codes propagate from daemon to gateway response", async () => {
    const deps = buildDeps();
    pair(deps);

    // Try to start without install → E_NOT_INSTALLED from daemon
    const res = await safeHandleCommand("telegram 100 req-1 /start openclaw", deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_NOT_INSTALLED");
    expect(res.message).toContain("not installed");
  });
});

describe("E2E: logs and upgrade", () => {
  test("logs after lifecycle actions", async () => {
    const deps = buildDeps();
    pair(deps);
    await safeHandleCommand("telegram 100 r1 /install openclaw", deps);
    await safeHandleCommand("telegram 100 r2 /start openclaw", deps);
    await safeHandleCommand("telegram 100 r3 /stop openclaw", deps);

    const res = await safeHandleCommand("telegram 100 r4 /logs openclaw 200", deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("log lines");
  });

  test("upgrade bumps version", async () => {
    const deps = buildDeps();
    pair(deps);
    await safeHandleCommand("telegram 100 r1 /install openclaw", deps);

    const res = await safeHandleCommand("telegram 100 r2 /upgrade openclaw", deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("0.1.0");
    expect(res.message).toContain("0.1.1");
    expect(res.message).toContain("backup");
  });

  test("agents list shows all agents", async () => {
    const deps = buildDeps();
    pair(deps);
    const res = await safeHandleCommand("telegram 100 r1 /agents", deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("1 agents");
  });

  test("agents list is sorted by health with lastError and drill-down guidance", async () => {
    const deps = buildDeps();
    pair(deps);
    const updatedAt = "2026-02-22T00:00:00.000Z";
    stubAgentList(deps, [
      {
        id: "a-healthy",
        name: "A Healthy",
        version: "1.0.0",
        installed: true,
        runtimeState: "running",
        health: "healthy",
        needsRemoteDiagnosis: false,
        updatedAt,
      },
      {
        id: "b-unhealthy",
        name: "B Unhealthy",
        version: "1.0.0",
        installed: true,
        runtimeState: "running",
        health: "unhealthy",
        needsRemoteDiagnosis: false,
        lastError: "connection timeout",
        updatedAt,
      },
      {
        id: "c-unknown",
        name: "C Unknown",
        version: "1.0.0",
        installed: false,
        runtimeState: "stopped",
        health: "unknown",
        needsRemoteDiagnosis: false,
        updatedAt,
      },
    ]);

    const res = await safeHandleCommand("telegram 100 r2 /agents", deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("listed 3 agents (2 installed)");
    expect(res.message).toContain('b-unhealthy: runtime=running health=unhealthy lastError="connection timeout"');
    expect(res.message).toContain("drill-down: /status <agent_id> | /logs <agent_id> [tail] | /diagnose <agent_id>");

    const unhealthyPos = res.message.indexOf("b-unhealthy: runtime=running health=unhealthy");
    const unknownPos = res.message.indexOf("c-unknown: runtime=stopped health=unknown");
    const healthyPos = res.message.indexOf("a-healthy: runtime=running health=healthy");
    expect(unhealthyPos).toBeGreaterThan(-1);
    expect(unknownPos).toBeGreaterThan(-1);
    expect(healthyPos).toBeGreaterThan(-1);
    expect(unhealthyPos).toBeLessThan(unknownPos);
    expect(unknownPos).toBeLessThan(healthyPos);
  });
});

describe("E2E: multi-provider support", () => {
  test("discord provider works", async () => {
    const deps = buildDeps();
    deps.sessions.registerPairCode("dc-code", 300);
    const res = await safeHandleCommand("discord guild-1 req-1 /pair dc-code", deps);
    expect(res.result).toBe("ok");

    const status = await safeHandleCommand("discord guild-1 req-2 /status", deps);
    expect(status.result).toBe("ok");
  });

  test("feishu provider works", async () => {
    const deps = buildDeps();
    deps.sessions.registerPairCode("fs-code", 300);
    const res = await safeHandleCommand("feishu chat-1 req-1 /pair fs-code", deps);
    expect(res.result).toBe("ok");

    const status = await safeHandleCommand("feishu chat-1 req-2 /status", deps);
    expect(status.result).toBe("ok");
  });
});
