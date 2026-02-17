import { describe, expect, test } from "bun:test";
import { ProviderSetupStore, type ProviderType } from "./setup";

describe("ProviderSetupStore", () => {
  test("starts unconfigured", () => {
    const store = new ProviderSetupStore();
    expect(store.isConfigured()).toBe(false);
    expect(store.getConfig()).toBeNull();
    expect(store.getProvider()).toBeNull();
  });

  test("configure sets config and returns it", () => {
    const store = new ProviderSetupStore();
    const config = store.configure("telegram", "my-token", "my-secret");

    expect(config.provider).toBe("telegram");
    expect(config.token).toBe("my-token");
    expect(config.webhook_secret).toBe("my-secret");
    expect(config.configured_at).toBeDefined();
    expect(store.isConfigured()).toBe(true);
  });

  test("getConfig returns the stored config", () => {
    const store = new ProviderSetupStore();
    store.configure("discord", "disc-token");
    const config = store.getConfig();
    expect(config).not.toBeNull();
    expect(config!.provider).toBe("discord");
    expect(config!.token).toBe("disc-token");
    expect(config!.webhook_secret).toBeUndefined();
  });

  test("getProvider returns provider type", () => {
    const store = new ProviderSetupStore();
    store.configure("feishu");
    expect(store.getProvider()).toBe("feishu");
  });

  test("configure overwrites previous config", () => {
    const store = new ProviderSetupStore();
    store.configure("telegram", "old-token");
    store.configure("discord", "new-token");
    expect(store.getProvider()).toBe("discord");
    expect(store.getConfig()!.token).toBe("new-token");
  });

  test("dummy provider logs and configures", () => {
    const store = new ProviderSetupStore();
    const config = store.configure("dummy");
    expect(config.provider).toBe("dummy");
    expect(store.isConfigured()).toBe(true);
    expect(config.configured_at).toBeDefined();
  });

  test("configure without optional params", () => {
    const store = new ProviderSetupStore();
    const config = store.configure("telegram");
    expect(config.token).toBeUndefined();
    expect(config.webhook_secret).toBeUndefined();
  });

  test("configured_at is a valid ISO string", () => {
    const store = new ProviderSetupStore();
    const config = store.configure("telegram", "tok");
    const date = new Date(config.configured_at);
    expect(date.getTime()).not.toBeNaN();
  });

  test("all provider types work", () => {
    const types: ProviderType[] = ["telegram", "discord", "feishu", "dummy"];
    for (const p of types) {
      const store = new ProviderSetupStore();
      const config = store.configure(p);
      expect(config.provider).toBe(p);
      expect(store.getProvider()).toBe(p);
    }
  });
});
