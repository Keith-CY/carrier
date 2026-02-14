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

    test("returns 0 when nothing to clean", () => {
      const store = new SessionStore(() => new Date("2026-01-01T00:00:00Z"));
      expect(store.cleanup()).toBe(0);
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
});
