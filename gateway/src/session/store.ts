import type {
  PairRequest,
  SessionRecord,
} from "../contracts/session";
import type { Provider } from "../contracts/commands";
import { existsSync, readFileSync } from "node:fs";
import { mkdir, rename } from "node:fs/promises";
import { dirname } from "node:path";

type PairingCodeRecord = {
  code: string;
  expiresAt: string;
};

function sessionKey(provider: Provider, chatId: string): string {
  return `${provider}:${chatId}`;
}

export class SessionStore {
  private readonly pairCodes = new Map<string, PairingCodeRecord>();
  private readonly sessions = new Map<string, SessionRecord>();
  private cleanupIntervalId: Timer | null = null;
  private savePromise: Promise<void> | null = null;

  constructor(
    private readonly now: () => Date = () => new Date(),
    private readonly sessionTTLSeconds: number = 30 * 24 * 60 * 60, // 30 days default
    private readonly persistencePath?: string
  ) {
    if (this.persistencePath) {
      this.loadSessions();
    }
  }

  issuePairCode(ttlSeconds = 300): PairingCodeRecord {
    const code = `pair-${crypto.randomUUID()}`;
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
    this.scheduleSave();
    return session;
  }

  createSession(request: { provider: Provider; chatId: string }): SessionRecord {
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
    this.scheduleSave();
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

    const lastSeenAt = this.now().toISOString();
    if (session.lastSeenAt !== lastSeenAt) {
      this.sessions.set(key, { ...session, lastSeenAt });
      this.scheduleSave();
    }
  }

  /** Remove expired pairing codes and stale sessions from the internal map. */
  cleanup(): number {
    let removed = 0;
    const nowMs = this.now().getTime();
    
    // Clean up expired pairing codes
    for (const [key, record] of this.pairCodes) {
      if (Date.parse(record.expiresAt) <= nowMs) {
        this.pairCodes.delete(key);
        removed += 1;
      }
    }
    
    // Clean up stale sessions (not seen within TTL)
    const sessionCutoff = nowMs - this.sessionTTLSeconds * 1000;
    let sessionsRemoved = 0;
    for (const [key, session] of this.sessions) {
      if (Date.parse(session.lastSeenAt) <= sessionCutoff) {
        this.sessions.delete(key);
        removed += 1;
        sessionsRemoved += 1;
      }
    }
    
    // If any sessions were removed, persist the updated state
    if (sessionsRemoved > 0) {
      this.scheduleSave();
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

  /** Start periodic cleanup of expired codes and stale sessions. */
  startPeriodicCleanup(intervalMs = 60 * 60 * 1000): this {
    this.stopPeriodicCleanup();
    this.cleanupIntervalId = setInterval(() => {
      this.cleanup();
    }, intervalMs);
    return this;
  }

  /** Stop the periodic cleanup interval. */
  stopPeriodicCleanup(): void {
    if (this.cleanupIntervalId !== null) {
      clearInterval(this.cleanupIntervalId);
      this.cleanupIntervalId = null;
    }
  }

  /** Wait for any pending save operation to complete. */
  async flush(): Promise<void> {
    if (this.savePromise !== null) {
      await this.savePromise;
    }
  }

  private loadSessions(): void {
    if (!this.persistencePath || !existsSync(this.persistencePath)) {
      return;
    }

    try {
      const data = readFileSync(this.persistencePath, "utf-8");
      if (data.trim() === "") {
        return;
      }
      const parsed = JSON.parse(data);
      
      if (Array.isArray(parsed)) {
        for (const record of parsed) {
          if (!this.isValidSessionRecord(record)) {
            console.warn("Skipping malformed session record:", record);
            continue;
          }
          this.sessions.set(sessionKey(record.provider, record.chatId), record);
        }
      } else {
        console.warn("Session persistence file does not contain an array, starting fresh");
      }
    } catch (error) {
      // If the file is corrupt or invalid, start fresh
      console.error("Failed to load sessions from disk:", error);
    }
  }

  private isValidSessionRecord(record: unknown): record is SessionRecord {
    if (typeof record !== "object" || record === null) {
      return false;
    }
    const r = record as Record<string, unknown>;
    return (
      typeof r.provider === "string" &&
      typeof r.chatId === "string" &&
      typeof r.sessionToken === "string" &&
      typeof r.createdAt === "string" &&
      typeof r.lastSeenAt === "string"
    );
  }

  private scheduleSave(): void {
    if (!this.persistencePath) {
      return;
    }

    // Debounce saves to avoid excessive I/O
    if (this.savePromise !== null) {
      return;
    }

    this.savePromise = this.saveSessions().finally(() => {
      this.savePromise = null;
    });
  }

  private async saveSessions(): Promise<void> {
    if (!this.persistencePath) {
      return;
    }

    try {
      // Ensure the directory exists
      const dir = dirname(this.persistencePath);
      await mkdir(dir, { recursive: true });

      // Convert sessions map to array
      const sessions = Array.from(this.sessions.values());
      const json = JSON.stringify(sessions, null, 2);

      // Atomic write: write to temp file, then rename
      const tempPath = `${this.persistencePath}.tmp`;
      await Bun.write(tempPath, json);
      await rename(tempPath, this.persistencePath);
    } catch (error) {
      console.error("Failed to save sessions to disk:", error);
    }
  }

  private issueSessionToken(): string {
    return `session-${crypto.randomUUID()}`;
  }
}
