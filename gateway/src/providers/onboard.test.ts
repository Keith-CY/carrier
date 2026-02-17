import { describe, expect, test } from "bun:test";
import { InMemoryDaemonClient } from "../daemon/client";
import { handleOnboardCommand, OnboardStore } from "./onboard";
import type { GatewayCommand } from "../contracts/commands";

function makeCmd(args: string[] = []): GatewayCommand {
  return {
    provider: "telegram",
    chatId: "chat-1",
    requestId: "req-1",
    sessionToken: "session-abc",
    name: "/onboard",
    args,
  };
}

function makeDeps() {
  return {
    daemon: new InMemoryDaemonClient(),
    onboardStore: new OnboardStore(),
  };
}

describe("handleOnboardCommand", () => {
  test("no args shows welcome with agent list", async () => {
    const deps = makeDeps();
    const res = await handleOnboardCommand(makeCmd(), deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("Welcome to Carrier");
    expect(res.message).toContain("OpenClaw");
  });

  test("env sets a variable", async () => {
    const deps = makeDeps();
    const res = await handleOnboardCommand(makeCmd(["env", "OPENAI_API_KEY=sk-xxx"]), deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("OPENAI_API_KEY set");
    expect(deps.onboardStore.getEnv("telegram:chat-1").get("OPENAI_API_KEY")).toBe("sk-xxx");
  });

  test("env without value returns usage error", async () => {
    const deps = makeDeps();
    const res = await handleOnboardCommand(makeCmd(["env"]), deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("install installs and starts agent", async () => {
    const deps = makeDeps();
    const res = await handleOnboardCommand(makeCmd(["install", "openclaw"]), deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("openclaw installed and running (healthy)");
  });

  test("install unknown agent returns error", async () => {
    const deps = makeDeps();
    const res = await handleOnboardCommand(makeCmd(["install", "nonexistent"]), deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_AGENT_NOT_FOUND");
  });

  test("install without agent_id returns usage error", async () => {
    const deps = makeDeps();
    const res = await handleOnboardCommand(makeCmd(["install"]), deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("status shows agent states", async () => {
    const deps = makeDeps();
    const res = await handleOnboardCommand(makeCmd(["status"]), deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("Agent status:");
    expect(res.message).toContain("OpenClaw");
  });

  test("unknown subcommand returns usage error", async () => {
    const deps = makeDeps();
    const res = await handleOnboardCommand(makeCmd(["unknown"]), deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("install reports partial success when start fails", async () => {
    const deps = makeDeps();
    deps.daemon.simulateStartProbeFailure("openclaw");
    const res = await handleOnboardCommand(makeCmd(["install", "openclaw"]), deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("installed but failed to start");
  });
});

describe("OnboardStore", () => {
  test("stores and retrieves env vars", () => {
    const store = new OnboardStore();
    store.setEnv("k1", "FOO", "bar");
    store.setEnv("k1", "BAZ", "qux");
    const vars = store.getEnv("k1");
    expect(vars.get("FOO")).toBe("bar");
    expect(vars.get("BAZ")).toBe("qux");
  });

  test("clearEnv removes all vars for key", () => {
    const store = new OnboardStore();
    store.setEnv("k1", "FOO", "bar");
    store.clearEnv("k1");
    expect(store.getEnv("k1").size).toBe(0);
  });

  test("getEnv returns empty map for unknown key", () => {
    const store = new OnboardStore();
    expect(store.getEnv("unknown").size).toBe(0);
  });
});
