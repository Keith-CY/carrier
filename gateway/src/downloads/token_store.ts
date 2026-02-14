import type { ReadOnlyDownloadToken } from "../contracts/session";

type DownloadTokenRecord = ReadOnlyDownloadToken & {
  consumedAt?: string;
};

export class DownloadTokenStore {
  private readonly tokens = new Map<string, DownloadTokenRecord>();

  constructor(private readonly now: () => Date = () => new Date()) {}

  issue(fileRef: string, ttlSeconds = 300, singleUse = true): ReadOnlyDownloadToken {
    const token = `dl-${crypto.randomUUID()}`;
    const expiresAt = new Date(this.now().getTime() + ttlSeconds * 1000).toISOString();
    const record: DownloadTokenRecord = {
      token,
      fileRef,
      expiresAt,
      singleUse,
    };
    this.tokens.set(token, record);
    return record;
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

  toDownloadURL(token: ReadOnlyDownloadToken): string {
    const fileName = token.fileRef.split("/").pop() || "artifact.zip";
    return `/downloads/${token.token}/${fileName}`;
  }
}
