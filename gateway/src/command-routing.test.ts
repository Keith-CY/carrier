import { describe, expect, test } from "bun:test";
import { handleCommand, parseInput, type GatewayDependencies } from "./index";
import { InMemoryDaemonClient } from "./daemon/client";
import { SessionStore } from "./session/store";
import { DownloadTokenStore } from "./downloads/token_store";

function buildDeps(): GatewayDependencies {
  return {
    daemon: new InMemoryDaemonClient(),
    sessions: new SessionStore(),
    downloads: new DownloadTokenStore(),
  };
}

function pairChat(deps: GatewayDependencies, provider: "telegram" | "discord" | "feishu" = "telegram", chatId = "100"): void {
  deps.sessions.registerPairCode("pair-ok", 300);
  deps.sessions.pair({ provider, chatId, code: "pair-ok" });
}

describe("command routing: /pair", () => {
  test("successful pairing returns session token", async () => {
    const deps = buildDeps();
    deps.sessions.registerPairCode("my-code", 300);

    const res = await handleCommand(parseInput("telegram 100 req-1 /pair my-code"), deps);

    expect(res.result).toBe("ok");
    expect(res.sessionToken).toBeString();
  });

  test("invalid code returns error", async () => {
    const deps = buildDeps();

    const res = await handleCommand(parseInput("telegram 100 req-1 /pair bad-code"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_PAIR_CODE_INVALID");
  });

  test("missing code returns usage error", async () => {
    const deps = buildDeps();

    const res = await handleCommand(parseInput("telegram 100 req-1 /pair"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });
});

describe("command routing: session requirement", () => {
  test("unpaired chat gets E_SESSION_REQUIRED", async () => {
    const deps = buildDeps();

    const res = await handleCommand(parseInput("telegram 100 req-1 /agents"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_SESSION_REQUIRED");
  });
});

describe("command routing: /agents", () => {
  test("lists agents for paired chat", async () => {
    const deps = buildDeps();
    pairChat(deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /agents"), deps);

    expect(res.result).toBe("ok");
    expect(res.message).toContain("listed");
  });
});

describe("command routing: /install", () => {
  test("missing agent returns usage error", async () => {
    const deps = buildDeps();
    pairChat(deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /install"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("installs agent successfully", async () => {
    const deps = buildDeps();
    pairChat(deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /install openclaw"), deps);

    expect(res.result).toBe("ok");
    expect(res.message).toContain("install completed");
  });
});

describe("command routing: /start", () => {
  test("start uninstalled agent returns error", async () => {
    const deps = buildDeps();
    pairChat(deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /start openclaw"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_NOT_INSTALLED");
  });

  test("start installed agent succeeds", async () => {
    const deps = buildDeps();
    pairChat(deps);
    await handleCommand(parseInput("telegram 100 req-0 /install openclaw"), deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /start openclaw"), deps);

    expect(res.result).toBe("ok");
  });

  test("start already-running agent returns error", async () => {
    const deps = buildDeps();
    pairChat(deps);
    await handleCommand(parseInput("telegram 100 req-0 /install openclaw"), deps);
    await handleCommand(parseInput("telegram 100 req-0 /start openclaw"), deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /start openclaw"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_ALREADY_RUNNING");
  });
});

describe("command routing: /stop", () => {
  test("stop already-stopped agent returns error", async () => {
    const deps = buildDeps();
    pairChat(deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /stop openclaw"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_ALREADY_STOPPED");
  });

  test("stop running agent succeeds", async () => {
    const deps = buildDeps();
    pairChat(deps);
    await handleCommand(parseInput("telegram 100 req-0 /install openclaw"), deps);
    await handleCommand(parseInput("telegram 100 req-0 /start openclaw"), deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /stop openclaw"), deps);

    expect(res.result).toBe("ok");
  });
});

describe("command routing: /status", () => {
  test("returns status for paired chat", async () => {
    const deps = buildDeps();
    pairChat(deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /status openclaw"), deps);

    expect(res.result).toBe("ok");
    expect(res.message).toContain("status");
  });

  test("status with no agent id returns all", async () => {
    const deps = buildDeps();
    pairChat(deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /status"), deps);

    expect(res.result).toBe("ok");
  });
});

describe("command routing: /logs", () => {
  test("missing agent returns usage error", async () => {
    const deps = buildDeps();
    pairChat(deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /logs"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("returns logs for agent", async () => {
    const deps = buildDeps();
    pairChat(deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /logs openclaw 10"), deps);

    expect(res.result).toBe("ok");
  });
});

describe("command routing: /upgrade", () => {
  test("missing agent returns usage error", async () => {
    const deps = buildDeps();
    pairChat(deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /upgrade"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("upgrade succeeds", async () => {
    const deps = buildDeps();
    pairChat(deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /upgrade openclaw"), deps);

    expect(res.result).toBe("ok");
    expect(res.message).toContain("upgrade completed");
  });
});

describe("command routing: /diagnose", () => {
  test("missing agent returns usage error", async () => {
    const deps = buildDeps();
    pairChat(deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /diagnose"), deps);

    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("diagnose returns download url", async () => {
    const deps = buildDeps();
    pairChat(deps);

    const res = await handleCommand(parseInput("telegram 100 req-1 /diagnose openclaw"), deps);

    expect(res.result).toBe("ok");
    expect(res.downloadUrl).toContain("/downloads/");
  });
});

describe("parseInput edge cases", () => {
  test("unknown command throws", () => {
    expect(() => parseInput("telegram 100 req-1 /unknown")).toThrow();
  });

  test("too few parts throws", () => {
    expect(() => parseInput("telegram 100")).toThrow();
  });
});
