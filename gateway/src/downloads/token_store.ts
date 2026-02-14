import type { ReadOnlyDownloadToken } from "../contracts/session";

type DownloadTokenRecord = ReadOnlyDownloadToken & {
  consumedAt?: string;
};

export class DownloadTokenStore {
  private nextToken = 0;
  private readonly tokens = new Map<string, DownloadTokenRecord>();

  constructor(private readonly now: () => Date = () => new Date()) {}

  issue(fileRef: string, ttlSeconds = 300, singleUse = true): ReadOnlyDownloadToken {
    this.nextToken += 1;
    const token = `dl-${this.nextToken}`;
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
