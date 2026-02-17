import { describe, test, expect } from "bun:test";
import { OnboardStore, validateProvider, computeReadiness } from "./onboard";
import type { DaemonAgentState } from "../daemon/client";

describe("OnboardStore", () => {
  test("initial state has no provider", () => {
    const store = new OnboardStore();
    const state = store.getState();
    expect(state.provider).toBeNull();
    expect(state.provider_configured).toBe(false);
    expect(state.configured_at).toBeNull();
  });

  test("configure sets provider and env", () => {
    const store = new OnboardStore();
    store.configure("telegram", "bot-token", { OPENAI_API_KEY: "sk-xxx" });
    const state = store.getState();
    expect(state.provider).toBe("telegram");
    expect(state.provider_configured).toBe(true);
    expect(state.env.OPENAI_API_KEY).toBe("sk-xxx");
    expect(state.configured_at).toBeTruthy();
  });

  test("configure without token sets provider_configured false", () => {
    const store = new OnboardStore();
    store.configure("discord", undefined, {});
    expect(store.getState().provider_configured).toBe(false);
  });

  test("getState returns a copy", () => {
    const store = new OnboardStore();
    store.configure("telegram", "t", { A: "1" });
    const s1 = store.getState();
    s1.env.A = "modified";
    expect(store.getState().env.A).toBe("1");
  });
});

describe("validateProvider", () => {
  test("accepts valid providers", () => {
    expect(validateProvider("telegram")).toBe(true);
    expect(validateProvider("discord")).toBe(true);
    expect(validateProvider("feishu")).toBe(true);
    expect(validateProvider("dummy")).toBe(true);
  });

  test("rejects invalid provider", () => {
    expect(validateProvider("slack")).toBe(false);
    expect(validateProvider("")).toBe(false);
  });
});

describe("computeReadiness", () => {
  const makeAgent = (id: string, name: string): DaemonAgentState => ({
    id, name, version: "1.0", installState: "not_installed", runtimeState: "stopped",
    health: "unknown", ports: [], restartCount: 0, needsRemoteDiagnosis: false, updatedAt: new Date().toISOString(),
  });

  test("marks agent ready when all required env provided", () => {
    const agents = [makeAgent("openclaw", "OpenClaw")];
    const reqs = new Map([["openclaw", {
      agent_id: "openclaw", name: "OpenClaw", description: "AI assistant",
      capabilities: ["chat"], env: { required: [{ name: "OPENAI_API_KEY" }], optional: [] },
    }]]);
    const result = computeReadiness(agents, reqs, { OPENAI_API_KEY: "sk-xxx" });
    expect(result).toHaveLength(1);
    expect(result[0].ready).toBe(true);
    expect(result[0].missing_env).toEqual([]);
  });

  test("marks agent not ready when required env missing", () => {
    const agents = [makeAgent("openclaw", "OpenClaw")];
    const reqs = new Map([["openclaw", {
      agent_id: "openclaw", name: "OpenClaw", description: "AI assistant",
      capabilities: ["chat"], env: { required: [{ name: "OPENAI_API_KEY" }], optional: [] },
    }]]);
    const result = computeReadiness(agents, reqs, {});
    expect(result[0].ready).toBe(false);
    expect(result[0].missing_env).toEqual(["OPENAI_API_KEY"]);
  });

  test("agent without requirements is ready", () => {
    const agents = [makeAgent("zeroclaw", "ZeroClaw")];
    const result = computeReadiness(agents, new Map(), {});
    expect(result[0].ready).toBe(true);
  });
});
