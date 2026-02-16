import { beforeEach, describe, expect, test } from "bun:test";
import { handleCommand, parseInput, ParseError, type GatewayDependencies } from "./index";
import { InMemoryDaemonClient } from "./daemon/client";
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

function pairChat(d: GatewayDependencies, provider: "telegram" | "discord" | "feishu" = "telegram", chatId = "100"): string {
  if (d.daemon instanceof InMemoryDaemonClient) {
    d.daemon.registerPairCode("pair-ok");
  }
  const session = d.sessions.createSession({ provider, chatId });
  return session.sessionToken;
}

describe("command routing: /pair", () => {
  beforeEach(() => {
    deps = buildDeps();
  });

  test("successful pairing returns session token", async () => {
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("my-code");
    }

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
  let token: string;
  
  beforeEach(() => {
    deps = buildDeps();
    token = pairChat(deps);
  });

  test("lists agents for paired chat", async () => {
    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /agents`), deps);

    expect(res.result).toBe("ok");
    expect(res.message).toContain("listed");
  });
});

describe("command routing: /install", () => {
  let token: string;
  
  beforeEach(() => {
    deps = buildDeps();
    token = pairChat(deps);
  });

  test("missing agent returns usage error", async () => {
    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /install`), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("installs agent successfully and updates state", async () => {
    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /install openclaw`), deps);

    expect(res.result).toBe("ok");
    expect(res.message).toContain("install completed");

    // Verify agent state was updated
    const statuses = await deps.daemon.getStatus("openclaw", { requestId: "verify", actor: "test" });
    expect(statuses).toHaveLength(1);
    expect(statuses[0].installState).toBe("installed");
  });
});

describe("command routing: /start", () => {
  let token: string;
  
  beforeEach(() => {
    deps = buildDeps();
    token = pairChat(deps);
  });

  test("start uninstalled agent returns error", async () => {
    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /start openclaw`), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_NOT_INSTALLED");
  });

  test("start installed agent succeeds and agent becomes running", async () => {
    await handleCommand(parseInput(`telegram 100 req-0 ${token} /install openclaw`), deps);

    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /start openclaw`), deps);

    expect(res.result).toBe("ok");

    // Verify agent state
    const statuses = await deps.daemon.getStatus("openclaw", { requestId: "verify", actor: "test" });
    expect(statuses[0].runtimeState).toBe("running");
    expect(statuses[0].health).toBe("healthy");
  });

  test("start with port conflict returns E_PORT_CONFLICT", async () => {
    await handleCommand(parseInput(`telegram 100 req-0 ${token} /install openclaw`), deps);
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.simulatePortConflict("openclaw");
    }

    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /start openclaw`), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_PORT_CONFLICT");
  });

  test("start probe failure returns E_START_PROBE_FAILED", async () => {
    await handleCommand(parseInput(`telegram 100 req-0 ${token} /install openclaw`), deps);
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.simulateStartProbeFailure("openclaw");
    }

    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /start openclaw`), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_START_PROBE_FAILED");
  });

  test("start already-running agent returns error", async () => {
    await handleCommand(parseInput(`telegram 100 req-0 ${token} /install openclaw`), deps);
    await handleCommand(parseInput(`telegram 100 req-0 ${token} /start openclaw`), deps);

    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /start openclaw`), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_ALREADY_RUNNING");
  });
});

describe("command routing: /stop", () => {
  let token: string;
  
  beforeEach(() => {
    deps = buildDeps();
    token = pairChat(deps);
  });

  test("stop already-stopped agent returns error", async () => {
    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /stop openclaw`), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_ALREADY_STOPPED");
  });

  test("stop running agent succeeds and agent becomes stopped", async () => {
    await handleCommand(parseInput(`telegram 100 req-0 ${token} /install openclaw`), deps);
    await handleCommand(parseInput(`telegram 100 req-0 ${token} /start openclaw`), deps);

    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /stop openclaw`), deps);

    expect(res.result).toBe("ok");

    // Verify agent state
    const statuses = await deps.daemon.getStatus("openclaw", { requestId: "verify", actor: "test" });
    expect(statuses[0].runtimeState).toBe("stopped");
  });
});

describe("command routing: /status", () => {
  let token: string;
  
  beforeEach(() => {
    deps = buildDeps();
    token = pairChat(deps);
  });

  test("returns status for paired chat", async () => {
    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /status openclaw`), deps);

    expect(res.result).toBe("ok");
    expect(res.message).toContain("status");
  });

  test("status with no agent id returns all", async () => {
    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /status`), deps);

    expect(res.result).toBe("ok");
  });
});

describe("command routing: /logs", () => {
  let token: string;
  
  beforeEach(() => {
    deps = buildDeps();
    token = pairChat(deps);
  });

  test("missing agent returns usage error", async () => {
    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /logs`), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("returns logs for agent", async () => {
    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /logs openclaw 10`), deps);

    expect(res.result).toBe("ok");
  });
});

describe("command routing: /upgrade", () => {
  let token: string;
  
  beforeEach(() => {
    deps = buildDeps();
    token = pairChat(deps);
  });

  test("missing agent returns usage error", async () => {
    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /upgrade`), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("upgrade succeeds", async () => {
    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /upgrade openclaw`), deps);

    expect(res.result).toBe("ok");
    expect(res.message).toContain("upgrade completed");
  });
});

describe("command routing: /diagnose", () => {
  let token: string;
  
  beforeEach(() => {
    deps = buildDeps();
    token = pairChat(deps);
  });

  test("missing agent returns usage error", async () => {
    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /diagnose`), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("diagnose returns download url", async () => {
    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /diagnose openclaw`), deps);

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
