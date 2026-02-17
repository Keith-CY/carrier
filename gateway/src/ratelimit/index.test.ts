import { describe, expect, it } from "bun:test";
import { RateLimiter } from "./index";

describe("RateLimiter", () => {
  it("allows requests under the limit", () => {
    const limiter = new RateLimiter({ perSession: 3, global: 10, windowMs: 1000 });
    const r1 = limiter.check("s1", 1000);
    const r2 = limiter.check("s1", 1001);
    const r3 = limiter.check("s1", 1002);
    expect(r1.allowed).toBe(true);
    expect(r2.allowed).toBe(true);
    expect(r3.allowed).toBe(true);
  });

  it("blocks when per-session limit is exceeded", () => {
    const limiter = new RateLimiter({ perSession: 2, global: 100, windowMs: 1000 });
    limiter.check("s1", 1000);
    limiter.check("s1", 1001);
    const r = limiter.check("s1", 1002);
    expect(r.allowed).toBe(false);
    expect(r.errorCode).toBe("E_RATE_LIMITED");
    expect(r.message).toContain("per-session");
  });

  it("blocks when global limit is exceeded", () => {
    const limiter = new RateLimiter({ perSession: 100, global: 3, windowMs: 1000 });
    limiter.check("s1", 1000);
    limiter.check("s2", 1001);
    limiter.check("s3", 1002);
    const r = limiter.check("s4", 1003);
    expect(r.allowed).toBe(false);
    expect(r.errorCode).toBe("E_RATE_LIMITED");
    expect(r.message).toContain("global");
  });

  it("allows requests after window expires", () => {
    const limiter = new RateLimiter({ perSession: 2, global: 100, windowMs: 1000 });
    limiter.check("s1", 1000);
    limiter.check("s1", 1001);
    // Blocked within window
    expect(limiter.check("s1", 1002).allowed).toBe(false);
    // Allowed after window passes
    expect(limiter.check("s1", 2001).allowed).toBe(true);
  });

  it("treats requests at the exact cutoff boundary as expired", () => {
    const limiter = new RateLimiter({ perSession: 1, global: 100, windowMs: 1000 });
    expect(limiter.check("s1", 1000).allowed).toBe(true);
    expect(limiter.check("s1", 1999).allowed).toBe(false);
    expect(limiter.check("s1", 2000).allowed).toBe(true);
  });

  it("isolates sessions from each other", () => {
    const limiter = new RateLimiter({ perSession: 1, global: 100, windowMs: 1000 });
    limiter.check("s1", 1000);
    expect(limiter.check("s1", 1001).allowed).toBe(false);
    expect(limiter.check("s2", 1001).allowed).toBe(true);
  });

  it("clearSession removes session tracking", () => {
    const limiter = new RateLimiter({ perSession: 1, global: 100, windowMs: 60000 });
    limiter.check("s1", 1000);
    expect(limiter.check("s1", 1001).allowed).toBe(false);
    limiter.clearSession("s1");
    expect(limiter.check("s1", 1002).allowed).toBe(true);
  });

  it("reset clears all state", () => {
    const limiter = new RateLimiter({ perSession: 1, global: 1, windowMs: 60000 });
    limiter.check("s1", 1000);
    expect(limiter.check("s2", 1001).allowed).toBe(false);
    limiter.reset();
    expect(limiter.check("s2", 1002).allowed).toBe(true);
  });

  it("returns config via getConfig", () => {
    const limiter = new RateLimiter({ perSession: 5, global: 50, windowMs: 2000 });
    const cfg = limiter.getConfig();
    expect(cfg.perSession).toBe(5);
    expect(cfg.global).toBe(50);
    expect(cfg.windowMs).toBe(2000);
  });

  it("prunes expired session windows to avoid unbounded growth", () => {
    const limiter = new RateLimiter({ perSession: 1, global: 100, windowMs: 1000 });
    limiter.check("s1", 1000);
    limiter.check("s2", 1000);

    const internal = limiter as unknown as { sessionWindows: Map<string, number[]> };
    expect(internal.sessionWindows.size).toBe(2);

    limiter.check("fresh", 2501);
    expect(internal.sessionWindows.size).toBe(1);
    expect(internal.sessionWindows.has("fresh")).toBe(true);
  });
});
