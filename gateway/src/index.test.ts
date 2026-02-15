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

function pairTelegramChat(deps: GatewayDependencies, chatId = "100"): string {
  deps.sessions.registerPairCode("pair-ok", 300);
  const session = deps.sessions.pair({ provider: "telegram", chatId, code: "pair-ok" });
  return session?.sessionToken ?? "";
}

describe("gateway parseInput", () => {
  test("parses telegram command", () => {
    const cmd = parseInput("telegram 100 req-1 /diagnose-consent openclaw yes");
    expect(cmd.provider).toBe("telegram");
    expect(cmd.chatId).toBe("100");
    expect(cmd.requestId).toBe("req-1");
    expect(cmd.name).toBe("/diagnose-consent");
    expect(cmd.args).toEqual(["openclaw", "yes"]);
  });

  test("parses discord command", () => {
    const cmd = parseInput("discord thread-22 req-2 /status openclaw");
    expect(cmd.provider).toBe("discord");
    expect(cmd.name).toBe("/status");
    expect(cmd.args).toEqual(["openclaw"]);
  });

  test("parses feishu command", () => {
    const cmd = parseInput("feishu chat-9 req-3 /logs openclaw 200");
    expect(cmd.provider).toBe("feishu");
    expect(cmd.name).toBe("/logs");
    expect(cmd.args).toEqual(["openclaw", "200"]);
  });

  test("returns parse error for unknown command", () => {
    expect(() => parseInput("telegram 100 req-1 /foobar")).toThrow(
      /unknown command:.*requestId=req-1/,
    );
  });
});

describe("gateway diagnose-consent routing", () => {
  test("returns usage error when agent is missing", async () => {
    const deps = buildDeps();
    const token = pairTelegramChat(deps);

    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /diagnose-consent`), deps);
    expect(res.result).toBe("error");
    expect(res.message).toContain("usage: /diagnose-consent");
  });

  test("returns validation error on invalid consent flag", async () => {
    const deps = buildDeps();
    const token = pairTelegramChat(deps);

    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /diagnose-consent openclaw maybe`), deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_CONSENT_FLAG_INVALID");
  });

  test("maps not-needed error with code", async () => {
    const deps = buildDeps();
    const token = pairTelegramChat(deps);

    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /diagnose-consent openclaw yes`), deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_REMOTE_DIAG_NOT_NEEDED");
  });

  test("returns handoff payload on success", async () => {
    const deps = buildDeps();
    const token = pairTelegramChat(deps);

    const daemon = deps.daemon as InMemoryDaemonClient;
    daemon.setDiagnoseArtifact("openclaw", "/tmp/openclaw.zip");
    daemon.setRemoteDiagnosisState("openclaw", true);

    const res = await handleCommand(parseInput(`telegram 100 req-1 ${token} /diagnose-consent openclaw yes`), deps);
    expect(res.result).toBe("ok");
    expect(res.handoffId).toBeDefined();
    expect(res.handoffStatus).toBe("pending");
    expect(res.downloadUrl).toContain("/downloads/");
  });
});
