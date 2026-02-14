import type {
  PairRequest,
  SessionRecord,
} from "../contracts/session";
import type { Provider } from "../contracts/commands";

type PairingCodeRecord = {
  code: string;
  expiresAt: string;
};

function sessionKey(provider: Provider, chatId: string): string {
  return `${provider}:${chatId}`;
}

export class SessionStore {
  private nextCode = 0;
  private nextToken = 0;
  private readonly pairCodes = new Map<string, PairingCodeRecord>();
  private readonly sessions = new Map<string, SessionRecord>();

  constructor(private readonly now: () => Date = () => new Date()) {}

  issuePairCode(ttlSeconds = 300): PairingCodeRecord {
    this.nextCode += 1;
    const code = `pair-${this.nextCode}`;
    const expiresAt = new Date(this.now().getTime() + ttlSeconds * 1000).toISOString();
    const record: PairingCodeRecord = { code, expiresAt };
    this.pairCodes.set(code, record);
    return record;
  }

  registerPairCode(code: string, ttlSeconds = 300): PairingCodeRecord {
    const expiresAt = new Date(this.now().getTime() + ttlSeconds * 1000).toISOString();
    const record: PairingCodeRecord = { code, expiresAt };
    this.pairCodes.set(code, record);
    return record;
  }

  pair(request: PairRequest): SessionRecord | null {
    const codeRecord = this.pairCodes.get(request.code);
    if (!codeRecord) {
      return null;
    }
    if (Date.parse(codeRecord.expiresAt) <= this.now().getTime()) {
      this.pairCodes.delete(request.code);
      return null;
    }

    this.pairCodes.delete(request.code);
    const existing = this.getSession(request.provider, request.chatId);
    const createdAt = existing?.createdAt ?? this.now().toISOString();
    const sessionToken = existing?.sessionToken ?? this.issueSessionToken();
    const session: SessionRecord = {
      provider: request.provider,
      chatId: request.chatId,
      sessionToken,
      createdAt,
      lastSeenAt: this.now().toISOString(),
    };
    this.sessions.set(sessionKey(request.provider, request.chatId), session);
    return session;
  }

  getSession(provider: Provider, chatId: string): SessionRecord | null {
    return this.sessions.get(sessionKey(provider, chatId)) ?? null;
  }

  touch(provider: Provider, chatId: string): void {
    const key = sessionKey(provider, chatId);
    const session = this.sessions.get(key);
    if (!session) {
      return;
    }
    this.sessions.set(key, { ...session, lastSeenAt: this.now().toISOString() });
  }

  /** Remove expired pairing codes from the internal map. */
  cleanup(): number {
    let removed = 0;
    const nowMs = this.now().getTime();
    for (const [key, record] of this.pairCodes) {
      if (Date.parse(record.expiresAt) <= nowMs) {
        this.pairCodes.delete(key);
        removed += 1;
      }
    }
    return removed;
  }

  /** Return the number of active pairing codes. */
  get pairCodeCount(): number {
    return this.pairCodes.size;
  }

  /** Return the number of active sessions. */
  get sessionCount(): number {
    return this.sessions.size;
  }

  private issueSessionToken(): string {
    this.nextToken += 1;
    return `session-${this.nextToken}`;
  }
}
