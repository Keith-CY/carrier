import type { ReadOnlyDownloadToken } from "../contracts/session";

export type DownloadTokenIssueOptions = {
  onCleanup?: (fileRef: string, reason: "consumed" | "expired") => void;
};

type DownloadTokenRecord = ReadOnlyDownloadToken & {
  consumedAt?: string;
  onCleanup?: DownloadTokenIssueOptions["onCleanup"];
};

export class DownloadTokenStore {
  private readonly tokens = new Map<string, DownloadTokenRecord>();
  private cleanupTimer: ReturnType<typeof setInterval> | null = null;

  constructor(private readonly now: () => Date = () => new Date()) {}

  /** Start a periodic cleanup interval. Returns this for chaining. */
  startPeriodicCleanup(intervalMs = 60_000): this {
    this.stopPeriodicCleanup();
    this.cleanupTimer = setInterval(() => this.cleanup(), intervalMs);
    // Allow the process to exit even if the timer is still running.
    if (this.cleanupTimer && typeof this.cleanupTimer === "object" && "unref" in this.cleanupTimer) {
      this.cleanupTimer.unref();
    }
    return this;
  }

  /** Stop the periodic cleanup interval if running. */
  stopPeriodicCleanup(): void {
    if (this.cleanupTimer !== null) {
      clearInterval(this.cleanupTimer);
      this.cleanupTimer = null;
    }
  }

  issue(fileRef: string, ttlSeconds = 300, singleUse = true, options: DownloadTokenIssueOptions = {}): ReadOnlyDownloadToken {
    const token = `dl-${crypto.randomUUID()}`;
    const expiresAt = new Date(this.now().getTime() + ttlSeconds * 1000).toISOString();
    const record: DownloadTokenRecord = {
      token,
      fileRef,
      expiresAt,
      singleUse,
      onCleanup: options.onCleanup,
    };
    this.tokens.set(token, record);
    return {
      token: record.token,
      fileRef: record.fileRef,
      expiresAt: record.expiresAt,
      singleUse: record.singleUse,
    };
  }

  /** Remove expired and consumed single-use tokens from the internal map. */
  cleanup(): number {
    let removed = 0;
    const nowMs = this.now().getTime();
    for (const [key, record] of this.tokens) {
      const expired = Date.parse(record.expiresAt) <= nowMs;
      const consumed = record.singleUse && record.consumedAt !== undefined;
      if (expired || consumed) {
        this.tokens.delete(key);
        record.onCleanup?.(record.fileRef, consumed ? "consumed" : "expired");
        removed += 1;
      }
    }
    return removed;
  }

  /** Return the number of tokens currently stored. */
  get size(): number {
    return this.tokens.size;
  }

  consume(token: string): ReadOnlyDownloadToken | null {
    const record = this.tokens.get(token);
    if (!record) {
      return null;
    }
    if (Date.parse(record.expiresAt) <= this.now().getTime()) {
      this.tokens.delete(token);
      record.onCleanup?.(record.fileRef, "expired");
      return null;
    }
    if (record.singleUse && record.consumedAt) {
      return null;
    }

    if (record.singleUse) {
      this.tokens.set(token, { ...record, consumedAt: this.now().toISOString() });
    }
    return {
      token: record.token,
      fileRef: record.fileRef,
      expiresAt: record.expiresAt,
      singleUse: record.singleUse,
    };
  }

  finalizeConsumed(token: string): void {
    const record = this.tokens.get(token);
    if (!record || !record.singleUse || !record.consumedAt) {
      return;
    }
    this.tokens.delete(token);
    record.onCleanup?.(record.fileRef, "consumed");
  }

  toDownloadURL(token: ReadOnlyDownloadToken): string {
    const rawName = token.fileRef.split(/[\\/]/).pop() ?? "";
    const fileName = rawName.trim().length > 0 ? rawName : "artifact.zip";
    return `/downloads/${token.token}/${encodeURIComponent(fileName)}`;
  }
}
