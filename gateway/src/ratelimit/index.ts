/**
 * Rate limiting module using a sliding window algorithm.
 *
 * Supports per-session and global rate limiting.
 * Configure via environment variables:
 *   - CARRIER_RATE_LIMIT_PER_SESSION (default: 30 requests per window)
 *   - CARRIER_RATE_LIMIT_GLOBAL      (default: 200 requests per window)
 *   - CARRIER_RATE_LIMIT_WINDOW_MS   (default: 60000 = 1 minute)
 */

export interface RateLimitConfig {
  /** Max requests per session within the window */
  perSession: number;
  /** Max global requests within the window */
  global: number;
  /** Sliding window duration in ms */
  windowMs: number;
}

export interface RateLimitResult {
  allowed: boolean;
  errorCode?: string;
  message?: string;
}

export class RateLimiter {
  private readonly config: RateLimitConfig;
  private readonly sessionWindows: Map<string, number[]> = new Map();
  private globalWindow: number[] = [];
  private cleanupIntervalId: Timer | null = null;

  constructor(config?: Partial<RateLimitConfig>) {
    this.config = {
      perSession: config?.perSession ?? envInt("CARRIER_RATE_LIMIT_PER_SESSION", 30),
      global: config?.global ?? envInt("CARRIER_RATE_LIMIT_GLOBAL", 200),
      windowMs: config?.windowMs ?? envInt("CARRIER_RATE_LIMIT_WINDOW_MS", 60_000),
    };
  }

  /**
   * Check whether a request from the given session is allowed.
   * If allowed, the request is recorded. If not, returns an error.
   */
  check(sessionKey: string, now: number = Date.now()): RateLimitResult {
    const cutoff = now - this.config.windowMs;

    // Prune and check global
    this.globalWindow = this.globalWindow.filter((t) => t > cutoff);
    if (this.globalWindow.length >= this.config.global) {
      return {
        allowed: false,
        errorCode: "E_RATE_LIMITED",
        message: `global rate limit exceeded (${this.config.global} req/${this.config.windowMs}ms)`,
      };
    }

    // Prune and check per-session
    let timestamps = this.sessionWindows.get(sessionKey) ?? [];
    timestamps = timestamps.filter((t) => t > cutoff);
    if (timestamps.length >= this.config.perSession) {
      this.sessionWindows.set(sessionKey, timestamps);
      return {
        allowed: false,
        errorCode: "E_RATE_LIMITED",
        message: `per-session rate limit exceeded (${this.config.perSession} req/${this.config.windowMs}ms)`,
      };
    }

    // Record
    timestamps.push(now);
    this.sessionWindows.set(sessionKey, timestamps);
    this.globalWindow.push(now);

    return { allowed: true };
  }

  /** Remove all tracked state for a session. */
  clearSession(sessionKey: string): void {
    this.sessionWindows.delete(sessionKey);
  }

  /** Reset all state. */
  reset(): void {
    this.sessionWindows.clear();
    this.globalWindow = [];
  }

  /** Remove expired windows from all sessions and global tracking. */
  cleanup(now: number = Date.now()): number {
    const cutoff = now - this.config.windowMs;
    let removed = 0;

    // Clean up global window
    this.globalWindow = this.globalWindow.filter((t) => t > cutoff);

    // Clean up session windows and remove empty sessions
    for (const [key, timestamps] of this.sessionWindows) {
      const filtered = timestamps.filter((t) => t > cutoff);
      if (filtered.length === 0) {
        this.sessionWindows.delete(key);
        removed += 1;
      } else {
        this.sessionWindows.set(key, filtered);
      }
    }

    return removed;
  }

  /** Start periodic cleanup of expired windows. */
  startPeriodicCleanup(intervalMs?: number): this {
    this.stopPeriodicCleanup();
    const interval = intervalMs ?? this.config.windowMs;
    this.cleanupIntervalId = setInterval(() => {
      this.cleanup();
    }, interval);
    return this;
  }

  /** Stop the periodic cleanup interval. */
  stopPeriodicCleanup(): void {
    if (this.cleanupIntervalId !== null) {
      clearInterval(this.cleanupIntervalId);
      this.cleanupIntervalId = null;
    }
  }

  /** Return current config (for inspection/testing). */
  getConfig(): Readonly<RateLimitConfig> {
    return { ...this.config };
  }
}

function envInt(name: string, fallback: number): number {
  const raw = typeof process !== "undefined" ? process.env?.[name] : undefined;
  if (!raw) return fallback;
  const parsed = Number.parseInt(raw, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}
