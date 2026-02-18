import { beforeEach, describe, expect, test } from "bun:test";
import { handleCommand, parseInput, type GatewayDependencies } from "./index";
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

describe("session token validation", () => {
  beforeEach(() => {
    deps = buildDeps();
  });

  test("parseInput extracts session token when present", () => {
    const cmd = parseInput("telegram 100 req-1 session-abc123 /agents");
    expect(cmd.sessionToken).toBe("session-abc123");
    expect(cmd.name).toBe("/agents");
    expect(cmd.args).toEqual([]);
  });

  test("parseInput treats non-session token as command name for backward compat", () => {
    const cmd = parseInput("telegram 100 req-1 /agents");
    expect(cmd.sessionToken).toBeUndefined();
    expect(cmd.name).toBe("/agents");
  });

  test("parseInput extracts session token with args", () => {
    const cmd = parseInput("telegram 100 req-1 session-xyz /install openclaw");
    expect(cmd.sessionToken).toBe("session-xyz");
    expect(cmd.name).toBe("/install");
    expect(cmd.args).toEqual(["openclaw"]);
  });

  test("commands without session token are rejected", async () => {
    pairChat(deps);
    const res = await handleCommand(parseInput("telegram 100 req-1 /agents"), deps);
    
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_SESSION_TOKEN_MISSING");
    expect(res.message).toContain("session token is required");
  });

  test("commands with wrong session token are rejected", async () => {
    pairChat(deps);
    const res = await handleCommand(
      parseInput("telegram 100 req-1 session-wrong /agents"),
      deps
    );
    
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_SESSION_TOKEN_INVALID");
    expect(res.message).toContain("session token is invalid");
  });

  test("commands with correct session token succeed", async () => {
    const sessionToken = pairChat(deps);
    const res = await handleCommand(
      parseInput(`telegram 100 req-1 ${sessionToken} /agents`),
      deps
    );
    
    expect(res.result).toBe("ok");
    expect(res.message).toContain("listed");
  });

  test("/pair does not require session token", async () => {
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("my-code");
    }
    
    const res = await handleCommand(parseInput("telegram 100 req-1 /pair my-code"), deps);
    
    expect(res.result).toBe("ok");
    expect(res.sessionToken).toBeString();
    expect(res.sessionToken).toMatch(/^session-/);
  });

  test("all command types require valid session token", async () => {
    const sessionToken = pairChat(deps);
    const commands = [
      "/agents",
      "/status",
      "/install openclaw",
      "/logs openclaw",
      "/upgrade openclaw",
    ];

    let reqId = 1;
    for (const cmdStr of commands) {
      // Without token - should fail
      const failRes = await handleCommand(
        parseInput(`telegram 100 req-${reqId++} ${cmdStr}`),
        deps
      );
      expect(failRes.result).toBe("error");
      expect(failRes.errorCode).toBe("E_SESSION_TOKEN_MISSING");

      // With correct token - should succeed (or fail with different error like E_NOT_INSTALLED)
      const successRes = await handleCommand(
        parseInput(`telegram 100 req-${reqId++} ${sessionToken} ${cmdStr}`),
        deps
      );
      expect(successRes.errorCode).not.toBe("E_SESSION_TOKEN_MISSING");
      expect(successRes.errorCode).not.toBe("E_SESSION_TOKEN_INVALID");
    }
  });

  test("session token validation works across different providers", async () => {
    const telegramToken = pairChat(deps, "telegram", "100");
    const discordToken = pairChat(deps, "discord", "200");
    
    // Telegram chat using Discord token should fail
    const res1 = await handleCommand(
      parseInput(`telegram 100 req-1 ${discordToken} /agents`),
      deps
    );
    expect(res1.result).toBe("error");
    expect(res1.errorCode).toBe("E_SESSION_TOKEN_INVALID");
    
    // Discord chat using Telegram token should fail
    const res2 = await handleCommand(
      parseInput(`discord 200 req-2 ${telegramToken} /agents`),
      deps
    );
    expect(res2.result).toBe("error");
    expect(res2.errorCode).toBe("E_SESSION_TOKEN_INVALID");
    
    // Each with correct token should succeed
    const res3 = await handleCommand(
      parseInput(`telegram 100 req-3 ${telegramToken} /agents`),
      deps
    );
    expect(res3.result).toBe("ok");
    
    const res4 = await handleCommand(
      parseInput(`discord 200 req-4 ${discordToken} /agents`),
      deps
    );
    expect(res4.result).toBe("ok");
  });

  test("session token from re-pairing is reused and validated", async () => {
    if (deps.daemon instanceof InMemoryDaemonClient) {
      deps.daemon.registerPairCode("code1");
      deps.daemon.registerPairCode("code2");
    }
    
    // First pairing
    const res1 = await handleCommand(parseInput("telegram 100 req-1 /pair code1"), deps);
    expect(res1.result).toBe("ok");
    const token1 = res1.sessionToken!;
    
    // Second pairing (same provider+chatId)
    const res2 = await handleCommand(parseInput("telegram 100 req-2 /pair code2"), deps);
    expect(res2.result).toBe("ok");
    const token2 = res2.sessionToken!;
    
    // Token should be reused
    expect(token1).toBe(token2);
    
    // Old token should still work
    const res3 = await handleCommand(
      parseInput(`telegram 100 req-3 ${token1} /agents`),
      deps
    );
    expect(res3.result).toBe("ok");
  });
});
