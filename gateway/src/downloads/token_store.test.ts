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

  test("repeated startPeriodicCleanup() does not create duplicate intervals", async () => {
    let time = new Date("2026-01-01T00:00:00Z");
    const store = new DownloadTokenStore(() => time);

    store.issue("a.txt", 1, false); // expires in 1 second

    // Call startPeriodicCleanup multiple times
    store.startPeriodicCleanup(50);
    store.startPeriodicCleanup(50);
    store.startPeriodicCleanup(50);

    // Advance past expiry
    time = new Date("2026-01-01T00:00:02Z");

    // Wait for cleanup
    await new Promise((resolve) => setTimeout(resolve, 120));

    store.stopPeriodicCleanup();

    // Token should be cleaned up exactly once (size 0), not cause errors
    expect(store.size).toBe(0);
  });

  test("startPeriodicCleanup() replaces previous interval without leak", async () => {
    let time = new Date("2026-01-01T00:00:00Z");
    const store = new DownloadTokenStore(() => time);

    // Issue a token that will expire
    store.issue("a.txt", 1, false);
    time = new Date("2026-01-01T00:00:02Z");

    // Start cleanup three times rapidly — only the last should be active
    store.startPeriodicCleanup(30);
    store.startPeriodicCleanup(30);
    store.startPeriodicCleanup(30);

    // Wait long enough for multiple cycles if duplicates existed
    await new Promise((resolve) => setTimeout(resolve, 200));

    store.stopPeriodicCleanup();

    // The store should have cleaned up the expired token
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
});
