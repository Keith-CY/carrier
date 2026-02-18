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

describe("handleOnboardCommand — interactive flow", () => {
  test("no args shows welcome with agent list and starts session", async () => {
    const deps = makeDeps();
    const res = await handleOnboardCommand(makeCmd(), deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("Welcome to Carrier");
    expect(res.message).toContain("OpenClaw");
    expect(res.message).toContain("Reply with the agent name");
  });

  test("full interactive flow: select agent → env → confirm → install", async () => {
    const deps = makeDeps();

    // Step 1: Start session
    const r1 = await handleOnboardCommand(makeCmd(), deps);
    expect(r1.result).toBe("ok");
    expect(r1.message).toContain("Welcome");

    // Step 2: Select agent
    const r2 = await handleOnboardCommand(makeCmd(["openclaw"]), deps);
    expect(r2.result).toBe("ok");
    expect(r2.message).toContain("Selected agent");
    expect(r2.message).toContain("openclaw");

    // Step 3: Provide env var
    const r3 = await handleOnboardCommand(makeCmd(["OPENAI_API_KEY=sk-xxx"]), deps);
    expect(r3.result).toBe("ok");
    expect(r3.message).toContain("OPENAI_API_KEY set");

    // Step 4: Done with env vars
    const r4 = await handleOnboardCommand(makeCmd(["done"]), deps);
    expect(r4.result).toBe("ok");
    expect(r4.message).toContain("Ready to install");
    expect(r4.message).toContain("OPENAI_API_KEY");

    // Step 5: Confirm
    const r5 = await handleOnboardCommand(makeCmd(["yes"]), deps);
    expect(r5.result).toBe("ok");
    expect(r5.message).toContain("openclaw installed and running");
  });

  test("select agent then skip env vars", async () => {
    const deps = makeDeps();
    await handleOnboardCommand(makeCmd(), deps);
    await handleOnboardCommand(makeCmd(["openclaw"]), deps);
    const r = await handleOnboardCommand(makeCmd(["done"]), deps);
    expect(r.result).toBe("ok");
    expect(r.message).toContain("No environment variables set");
  });

  test("select unknown agent returns error", async () => {
    const deps = makeDeps();
    await handleOnboardCommand(makeCmd(), deps);
    const res = await handleOnboardCommand(makeCmd(["nonexistent"]), deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_AGENT_NOT_FOUND");
  });

  test("cancel aborts active session", async () => {
    const deps = makeDeps();
    await handleOnboardCommand(makeCmd(), deps);
    await handleOnboardCommand(makeCmd(["openclaw"]), deps);
    const res = await handleOnboardCommand(makeCmd(["cancel"]), deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("cancelled");
  });

  test("cancel with no active session is a no-op", async () => {
    const deps = makeDeps();
    const res = await handleOnboardCommand(makeCmd(["cancel"]), deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("No active");
  });

  test("confirmation 'no' goes back to env step", async () => {
    const deps = makeDeps();
    await handleOnboardCommand(makeCmd(), deps);
    await handleOnboardCommand(makeCmd(["openclaw"]), deps);
    await handleOnboardCommand(makeCmd(["done"]), deps);
    const res = await handleOnboardCommand(makeCmd(["no"]), deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("Going back");
  });

  test("invalid confirmation returns usage error", async () => {
    const deps = makeDeps();
    await handleOnboardCommand(makeCmd(), deps);
    await handleOnboardCommand(makeCmd(["openclaw"]), deps);
    await handleOnboardCommand(makeCmd(["done"]), deps);
    const res = await handleOnboardCommand(makeCmd(["maybe"]), deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("invalid env var format returns usage error", async () => {
    const deps = makeDeps();
    await handleOnboardCommand(makeCmd(), deps);
    await handleOnboardCommand(makeCmd(["openclaw"]), deps);
    const res = await handleOnboardCommand(makeCmd(["not-a-keyvalue"]), deps);
    expect(res.result).toBe("error");
    expect(res.errorCode).toBe("E_USAGE");
  });

  test("install reports partial success when start fails", async () => {
    const deps = makeDeps();
    deps.daemon.simulateStartProbeFailure("openclaw");
    await handleOnboardCommand(makeCmd(), deps);
    await handleOnboardCommand(makeCmd(["openclaw"]), deps);
    await handleOnboardCommand(makeCmd(["done"]), deps);
    const res = await handleOnboardCommand(makeCmd(["yes"]), deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("installed but failed to start");
  });

  test("status works anytime as standalone command", async () => {
    const deps = makeDeps();
    const res = await handleOnboardCommand(makeCmd(["status"]), deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("Agent status:");
    expect(res.message).toContain("OpenClaw");
  });

  test("agent shortcut without starting session first", async () => {
    const deps = makeDeps();
    // Directly provide agent name without `/onboard` first
    const res = await handleOnboardCommand(makeCmd(["openclaw"]), deps);
    expect(res.result).toBe("ok");
    expect(res.message).toContain("Selected agent");
  });

  test("multiple env vars accumulate", async () => {
    const deps = makeDeps();
    await handleOnboardCommand(makeCmd(), deps);
    await handleOnboardCommand(makeCmd(["openclaw"]), deps);
    await handleOnboardCommand(makeCmd(["FOO=bar"]), deps);
    await handleOnboardCommand(makeCmd(["BAZ=qux"]), deps);
    const r = await handleOnboardCommand(makeCmd(["done"]), deps);
    expect(r.message).toContain("FOO");
    expect(r.message).toContain("BAZ");
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

  test("session lifecycle", () => {
    const store = new OnboardStore();
    expect(store.hasActiveSession("k1")).toBe(false);
    store.startSession("k1");
    expect(store.hasActiveSession("k1")).toBe(false); // idle is not active
    store.updateSession("k1", { step: "agent_selected", selectedAgent: "openclaw" });
    expect(store.hasActiveSession("k1")).toBe(true);
    store.clearSession("k1");
    expect(store.hasActiveSession("k1")).toBe(false);
  });

  test("getSession returns undefined for unknown key", () => {
    const store = new OnboardStore();
    expect(store.getSession("unknown")).toBeUndefined();
  });
});
