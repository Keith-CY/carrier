import { describe, expect, test } from "bun:test";
import { DownloadTokenStore } from "./token_store";

describe("DownloadTokenStore", () => {
  const createStoreAt = (isoTime: string) => new DownloadTokenStore(() => new Date(isoTime));

  test("issue() returns a valid token with correct fileRef, expiresAt, singleUse", () => {
    const base = new Date("2026-01-01T00:00:00Z");
    const store = new DownloadTokenStore(() => base);

    const tok = store.issue("builds/app.zip", 600, true);

    expect(tok.token).toBeString();
    expect(tok.fileRef).toBe("builds/app.zip");
    expect(tok.singleUse).toBe(true);
    expect(tok.expiresAt).toBe(new Date(base.getTime() + 600_000).toISOString());
  });

  test("consume() valid token returns the record", () => {
    const store = createStoreAt("2026-01-01T00:00:00Z");
    const tok = store.issue("file.txt", 300, false);

    const result = store.consume(tok.token);

    expect(result).not.toBeNull();
    expect(result!.fileRef).toBe("file.txt");
    expect(result!.token).toBe(tok.token);
  });

  test("consume() expired token returns null", () => {
    let time = new Date("2026-01-01T00:00:00Z");
    const store = new DownloadTokenStore(() => time);

    const tok = store.issue("file.txt", 60, false);

    // Advance past expiry
    time = new Date("2026-01-01T00:02:00Z");

    expect(store.consume(tok.token)).toBeNull();
  });

  test("consume() unknown token returns null", () => {
    const store = createStoreAt("2026-01-01T00:00:00Z");

    expect(store.consume("not-a-real-token")).toBeNull();
  });

  test("single-use token consumed twice → second returns null", () => {
    const store = new DownloadTokenStore(() => new Date("2026-01-01T00:00:00Z"));
    const tok = store.issue("file.txt", 300, true);

    expect(store.consume(tok.token)).not.toBeNull();
    expect(store.consume(tok.token)).toBeNull();
  });

  test("cleanup() removes expired tokens", () => {
    let time = new Date("2026-01-01T00:00:00Z");
    const store = new DownloadTokenStore(() => time);

    store.issue("a.txt", 60, false);
    store.issue("b.txt", 600, false);

    // Advance past first token's expiry but not second
    time = new Date("2026-01-01T00:02:00Z");

    const removed = store.cleanup();
    expect(removed).toBe(1);
    expect(store.size).toBe(1);
  });

  test("cleanup() removes consumed single-use tokens", () => {
    const store = new DownloadTokenStore(() => new Date("2026-01-01T00:00:00Z"));

    const tok = store.issue("file.txt", 300, true);
    store.issue("other.txt", 300, true); // not consumed

    store.consume(tok.token);

    const removed = store.cleanup();
    expect(removed).toBe(1);
    expect(store.size).toBe(1);
  });

  test("toDownloadURL() generates correct path format", () => {
    const store = new DownloadTokenStore();
    const tok = store.issue("some/path/artifact.zip");

    const url = store.toDownloadURL(tok);

    expect(url).toBe(`/downloads/${tok.token}/artifact.zip`);
  });

  test("toDownloadURL() uses full fileRef as filename when no slash exists", () => {
    const store = new DownloadTokenStore();
    const tok = store.issue("artifact.zip");

    const url = store.toDownloadURL(tok);

    expect(url).toBe(`/downloads/${tok.token}/artifact.zip`);
  });

  test("toDownloadURL() normalizes Windows separators and trims filename whitespace", () => {
    const store = new DownloadTokenStore();
    const tok = store.issue("builds\\nested\\  artifact.zip  ");

    const url = store.toDownloadURL(tok);

    expect(url).toBe(`/downloads/${tok.token}/artifact.zip`);
  });

  test("toDownloadURL() URL-encodes spaces and unicode characters", () => {
    const store = new DownloadTokenStore();
    const tok = store.issue("artifacts/report 2026 ✅.zip");

    const url = store.toDownloadURL(tok);

    expect(url).toBe(`/downloads/${tok.token}/${encodeURIComponent("report 2026 ✅.zip")}`);
  });

  test("toDownloadURL() falls back to artifact.zip when filename is empty/whitespace", () => {
    const store = new DownloadTokenStore();
    const tok = store.issue("artifacts/   ");

    const url = store.toDownloadURL(tok);

    expect(url).toBe(`/downloads/${tok.token}/artifact.zip`);
  });

  test("startPeriodicCleanup() triggers cleanup on interval", async () => {
    let time = new Date("2026-01-01T00:00:00Z");
    const store = new DownloadTokenStore(() => time);

    store.issue("a.txt", 1, false); // expires in 1 second

    // Advance time past expiry
    time = new Date("2026-01-01T00:00:02Z");

    store.startPeriodicCleanup(50); // 50ms interval for fast test

    // Wait for at least one cleanup cycle
    await new Promise((resolve) => setTimeout(resolve, 120));

    store.stopPeriodicCleanup();
    expect(store.size).toBe(0);
  });

  test("stopPeriodicCleanup() stops the interval", async () => {
    let time = new Date("2026-01-01T00:00:00Z");
    const store = new DownloadTokenStore(() => time);

    store.startPeriodicCleanup(50);
    store.stopPeriodicCleanup();

    store.issue("a.txt", 1, false);
    time = new Date("2026-01-01T00:00:02Z");

    await new Promise((resolve) => setTimeout(resolve, 120));

    // Token should still be there since cleanup was stopped
    expect(store.size).toBe(1);
  });

  test("startPeriodicCleanup() is idempotent and replaces the previous timer", () => {
    const originalSetInterval = globalThis.setInterval;
    const originalClearInterval = globalThis.clearInterval;

    const createdHandles: Array<{ id: number; unref: () => void }> = [];
    const clearedHandleIDs: number[] = [];

    try {
      globalThis.setInterval = ((_: () => void, __?: number) => {
        const handle = {
          id: createdHandles.length + 1,
          unref: () => undefined,
        };
        createdHandles.push(handle);
        return handle as unknown as ReturnType<typeof setInterval>;
      }) as typeof setInterval;

      globalThis.clearInterval = ((handle: ReturnType<typeof setInterval>) => {
        const id = (handle as unknown as { id?: number }).id;
        if (typeof id === "number") {
          clearedHandleIDs.push(id);
        }
      }) as typeof clearInterval;

      const store = new DownloadTokenStore();
      store.startPeriodicCleanup(1000);
      store.startPeriodicCleanup(1000);
      store.stopPeriodicCleanup();

      expect(createdHandles).toHaveLength(2);
      expect(clearedHandleIDs).toEqual([1, 2]);
    } finally {
      globalThis.setInterval = originalSetInterval;
      globalThis.clearInterval = originalClearInterval;
    }
  });

  test("stopPeriodicCleanup() is safe to call repeatedly", () => {
    const originalSetInterval = globalThis.setInterval;
    const originalClearInterval = globalThis.clearInterval;

    let clearCalls = 0;

    try {
      globalThis.setInterval = ((_: () => void, __?: number) => {
        return { unref: () => undefined } as unknown as ReturnType<typeof setInterval>;
      }) as typeof setInterval;

      globalThis.clearInterval = ((_: ReturnType<typeof setInterval>) => {
        clearCalls += 1;
      }) as typeof clearInterval;

      const store = new DownloadTokenStore();
      store.startPeriodicCleanup(1000);
      store.stopPeriodicCleanup();
      store.stopPeriodicCleanup();

      expect(clearCalls).toBe(1);
    } finally {
      globalThis.setInterval = originalSetInterval;
      globalThis.clearInterval = originalClearInterval;
    }
  });
});
