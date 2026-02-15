import { describe, expect, test } from "bun:test";
import { SessionStore } from "./store";

describe("SessionStore", () => {
  describe("issuePairCode", () => {
    test("returns a code with expiry", () => {
      const base = new Date("2026-01-01T00:00:00Z");
      const store = new SessionStore(() => base);

      const record = store.issuePairCode(120);

      expect(record.code).toBeString();
      expect(record.expiresAt).toBe(new Date(base.getTime() + 120_000).toISOString());
    });

    test("issues unique codes", () => {
      const store = new SessionStore();
      const a = store.issuePairCode();
      const b = store.issuePairCode();

      expect(a.code).not.toBe(b.code);
    });
  });

  describe("registerPairCode", () => {
    test("registers an externally supplied code", () => {
      const base = new Date("2026-01-01T00:00:00Z");
      const store = new SessionStore(() => base);

      const record = store.registerPairCode("my-code", 60);

      expect(record.code).toBe("my-code");
      expect(record.expiresAt).toBe(new Date(base.getTime() + 60_000).toISOString());
    });
  });

  describe("pair", () => {
    test("pairs successfully with valid code", () => {
      const store = new SessionStore(() => new Date("2026-01-01T00:00:00Z"));
      store.registerPairCode("valid-code", 300);

      const session = store.pair({ provider: "telegram", chatId: "chat-1", code: "valid-code" });

      expect(session).not.toBeNull();
      expect(session!.provider).toBe("telegram");
      expect(session!.chatId).toBe("chat-1");
      expect(session!.sessionToken).toBeString();
    });

    test("returns null for unknown code", () => {
      const store = new SessionStore();

      const session = store.pair({ provider: "telegram", chatId: "chat-1", code: "bad-code" });

      expect(session).toBeNull();
    });

    test("returns null for expired code", () => {
      let time = new Date("2026-01-01T00:00:00Z");
      const store = new SessionStore(() => time);
      store.registerPairCode("exp-code", 60);

      time = new Date("2026-01-01T00:02:00Z");

      const session = store.pair({ provider: "telegram", chatId: "chat-1", code: "exp-code" });

      expect(session).toBeNull();
    });

    test("returns null for code at exact expiry boundary", () => {
      let time = new Date("2026-01-01T00:00:00Z");
      const store = new SessionStore(() => time);
      store.registerPairCode("boundary-code", 60);

      // Advance to exactly the expiry moment (60s later)
      time = new Date("2026-01-01T00:01:00Z");

      const session = store.pair({ provider: "telegram", chatId: "chat-1", code: "boundary-code" });

      expect(session).toBeNull();
    });

    test("code is consumed after successful pairing", () => {
      const store = new SessionStore(() => new Date("2026-01-01T00:00:00Z"));
      store.registerPairCode("once", 300);

      store.pair({ provider: "telegram", chatId: "c1", code: "once" });
      const second = store.pair({ provider: "discord", chatId: "c2", code: "once" });

      expect(second).toBeNull();
    });

    test("re-pairing same provider+chatId reuses session token and preserves createdAt", () => {
      let time = new Date("2026-01-01T00:00:00Z");
      const store = new SessionStore(() => time);
      store.registerPairCode("code-1", 3600);

      const first = store.pair({ provider: "telegram", chatId: "c1", code: "code-1" });

      time = new Date("2026-01-01T00:30:00Z");
      store.registerPairCode("code-2", 3600);
      const second = store.pair({ provider: "telegram", chatId: "c1", code: "code-2" });

      expect(first!.sessionToken).toBe(second!.sessionToken);
      expect(second!.createdAt).toBe(first!.createdAt);
      expect(second!.lastSeenAt).toBe(time.toISOString());
      expect(second!.lastSeenAt).not.toBe(first!.lastSeenAt);
    });

    test("different providers get different session tokens", () => {
      const store = new SessionStore(() => new Date("2026-01-01T00:00:00Z"));
      store.registerPairCode("code-1", 300);
      store.registerPairCode("code-2", 300);

      const a = store.pair({ provider: "telegram", chatId: "c1", code: "code-1" });
      const b = store.pair({ provider: "discord", chatId: "c1", code: "code-2" });

      expect(a!.sessionToken).not.toBe(b!.sessionToken);
    });
  });

  describe("getSession", () => {
    test("returns null for unpaired chat", () => {
      const store = new SessionStore();

      expect(store.getSession("telegram", "unknown")).toBeNull();
    });

    test("returns session after pairing", () => {
      const store = new SessionStore(() => new Date("2026-01-01T00:00:00Z"));
      store.registerPairCode("ok", 300);
      store.pair({ provider: "discord", chatId: "ch", code: "ok" });

      const session = store.getSession("discord", "ch");

      expect(session).not.toBeNull();
      expect(session!.chatId).toBe("ch");
    });
  });

  describe("touch", () => {
    test("updates lastSeenAt", () => {
      let time = new Date("2026-01-01T00:00:00Z");
      const store = new SessionStore(() => time);
      store.registerPairCode("ok", 300);
      store.pair({ provider: "telegram", chatId: "c1", code: "ok" });

      time = new Date("2026-01-01T01:00:00Z");
      store.touch("telegram", "c1");

      const session = store.getSession("telegram", "c1");
      expect(session!.lastSeenAt).toBe(time.toISOString());
    });

    test("no-op for unknown session", () => {
      const store = new SessionStore();
      // Should not throw
      store.touch("telegram", "unknown");
    });
  });

  describe("cleanup", () => {
    test("removes expired pairing codes", () => {
      let time = new Date("2026-01-01T00:00:00Z");
      const store = new SessionStore(() => time);
      store.registerPairCode("code-a", 60);
      store.registerPairCode("code-b", 300);

      expect(store.pairCodeCount).toBe(2);

      // Advance past first code's expiry but before second
      time = new Date("2026-01-01T00:02:00Z");
      const removed = store.cleanup();

      expect(removed).toBe(1);
      expect(store.pairCodeCount).toBe(1);

      // The non-expired code should still work
      const session = store.pair({ provider: "telegram", chatId: "c1", code: "code-b" });
      expect(session).not.toBeNull();
    });

    test("removes session exactly at stale-session TTL boundary", () => {
      let time = new Date("2026-01-01T00:00:00Z");
      const store = new SessionStore(() => time, 60);
      store.registerPairCode("code-ttl", 300);
      store.pair({ provider: "telegram", chatId: "ttl-chat", code: "code-ttl" });

      // Exactly at TTL boundary should be treated as stale and removed.
      time = new Date("2026-01-01T00:01:00Z");
      const removed = store.cleanup();

      expect(removed).toBe(1);
      expect(store.getSession("telegram", "ttl-chat")).toBeNull();
    });

    test("returns 0 when nothing to clean", () => {
      const store = new SessionStore(() => new Date("2026-01-01T00:00:00Z"));
      expect(store.cleanup()).toBe(0);
    });
  });

  describe("periodic cleanup lifecycle", () => {
    test("startPeriodicCleanup and stopPeriodicCleanup are idempotent", () => {
      const store = new SessionStore();

      // Should not throw when repeatedly started/stopped.
      store.startPeriodicCleanup(10);
      store.startPeriodicCleanup(10);
      store.stopPeriodicCleanup();
      store.stopPeriodicCleanup();
    });
  });

  describe("sessionCount", () => {
    test("tracks active sessions", () => {
      const store = new SessionStore(() => new Date("2026-01-01T00:00:00Z"));
      expect(store.sessionCount).toBe(0);

      store.registerPairCode("code-1", 300);
      store.pair({ provider: "telegram", chatId: "c1", code: "code-1" });

      expect(store.sessionCount).toBe(1);
    });
  });

  describe("persistence", () => {
    const testDir = "/tmp/carrier-session-test";
    
    // Helper to generate unique path for each test
    function getTestPath(name: string): string {
      return `${testDir}/${name}-${Date.now()}-${Math.random()}.json`;
    }

    test("creates session and persists to disk", async () => {
      const testPath = getTestPath("create-persist");
      const store = new SessionStore(() => new Date("2026-01-01T00:00:00Z"), 30 * 24 * 60 * 60, testPath);
      
      store.createSession({ provider: "telegram", chatId: "persist-1" });
      
      // Wait for async save to complete
      await store.flush();

      // Verify file exists and contains the session
      const file = Bun.file(testPath);
      const content = await file.text();
      const sessions = JSON.parse(content);

      expect(Array.isArray(sessions)).toBe(true);
      expect(sessions.length).toBe(1);
      expect(sessions[0].provider).toBe("telegram");
      expect(sessions[0].chatId).toBe("persist-1");
    });

    test("loads sessions from disk on initialization", async () => {
      const testPath = getTestPath("load-persist");
      // First store creates and saves session
      const store1 = new SessionStore(() => new Date("2026-01-01T00:00:00Z"), 30 * 24 * 60 * 60, testPath);
      const session = store1.createSession({ provider: "discord", chatId: "persist-2" });
      await store1.flush();

      // Second store loads from disk
      const store2 = new SessionStore(() => new Date("2026-01-01T00:01:00Z"), 30 * 24 * 60 * 60, testPath);
      const loaded = store2.getSession("discord", "persist-2");

      expect(loaded).not.toBeNull();
      expect(loaded!.sessionToken).toBe(session.sessionToken);
      expect(loaded!.createdAt).toBe(session.createdAt);
    });

    test("handles missing persistence file gracefully", () => {
      const missingPath = "/tmp/carrier-session-test/nonexistent.json";
      // Should not throw when file doesn't exist
      const store = new SessionStore(() => new Date("2026-01-01T00:00:00Z"), 30 * 24 * 60 * 60, missingPath);
      expect(store.sessionCount).toBe(0);
    });

    test("handles corrupt persistence file gracefully", async () => {
      const corruptPath = `${testDir}/corrupt.json`;
      await Bun.write(corruptPath, "{ this is not valid json");
      
      // Should not throw, just start fresh
      const store = new SessionStore(() => new Date("2026-01-01T00:00:00Z"), 30 * 24 * 60 * 60, corruptPath);
      expect(store.sessionCount).toBe(0);
    });

    test("persists multiple sessions", async () => {
      const testPath = getTestPath("multi-persist");
      const store = new SessionStore(() => new Date("2026-01-01T00:00:00Z"), 30 * 24 * 60 * 60, testPath);
      
      store.createSession({ provider: "telegram", chatId: "multi-1" });
      store.createSession({ provider: "discord", chatId: "multi-2" });
      store.createSession({ provider: "telegram", chatId: "multi-3" });
      
      await store.flush();

      // Load in new store
      const store2 = new SessionStore(() => new Date("2026-01-01T00:00:00Z"), 30 * 24 * 60 * 60, testPath);
      expect(store2.sessionCount).toBe(3);
      expect(store2.getSession("telegram", "multi-1")).not.toBeNull();
      expect(store2.getSession("discord", "multi-2")).not.toBeNull();
      expect(store2.getSession("telegram", "multi-3")).not.toBeNull();
    });

    test("pairing persists to disk", async () => {
      const testPath = getTestPath("pair-persist");
      const store = new SessionStore(() => new Date("2026-01-01T00:00:00Z"), 30 * 24 * 60 * 60, testPath);
      
      store.registerPairCode("pair-persist", 300);
      const session = store.pair({ provider: "telegram", chatId: "paired-1", code: "pair-persist" });
      
      await store.flush();

      // Verify persistence
      const store2 = new SessionStore(() => new Date("2026-01-01T00:00:00Z"), 30 * 24 * 60 * 60, testPath);
      const loaded = store2.getSession("telegram", "paired-1");

      expect(loaded).not.toBeNull();
      expect(loaded!.sessionToken).toBe(session!.sessionToken);
    });

    test("touch persists updated lastSeenAt across reload", async () => {
      const testPath = getTestPath("touch-persist");
      let time = new Date("2026-01-01T00:00:00Z");
      const store = new SessionStore(() => time, 30 * 24 * 60 * 60, testPath);

      store.createSession({ provider: "telegram", chatId: "touch-1" });
      await store.flush();
      const before = store.getSession("telegram", "touch-1");

      time = new Date("2026-01-01T00:05:00Z");
      store.touch("telegram", "touch-1");
      await store.flush();

      const reloaded = new SessionStore(() => time, 30 * 24 * 60 * 60, testPath);
      const after = reloaded.getSession("telegram", "touch-1");

      expect(before).not.toBeNull();
      expect(after).not.toBeNull();
      expect(after!.lastSeenAt).toBe(time.toISOString());
      expect(after!.lastSeenAt).not.toBe(before!.lastSeenAt);
    });

    test("without persistence path, store works in-memory only", async () => {
      const store = new SessionStore(() => new Date("2026-01-01T00:00:00Z"));
      
      store.createSession({ provider: "telegram", chatId: "memory-only" });
      
      // flush should be a no-op
      await store.flush();
      
      // Session exists in memory
      expect(store.getSession("telegram", "memory-only")).not.toBeNull();
    });
  });
});
