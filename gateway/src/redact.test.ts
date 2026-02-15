import { describe, expect, test } from "bun:test";
import {
  REDACTED,
  isSensitiveKey,
  redactText,
  redactEnviron,
  redactErrorMessage,
} from "./redact";

describe("isSensitiveKey", () => {
  test("detects sensitive keys", () => {
    expect(isSensitiveKey("MY_API_KEY")).toBe(true);
    expect(isSensitiveKey("secret")).toBe(true);
    expect(isSensitiveKey("SESSION_TOKEN")).toBe(true);
    expect(isSensitiveKey("db_password")).toBe(true);
    expect(isSensitiveKey("CREDENTIAL_FILE")).toBe(true);
  });

  test("non-sensitive keys", () => {
    expect(isSensitiveKey("PORT")).toBe(false);
    expect(isSensitiveKey("HOST")).toBe(false);
    expect(isSensitiveKey("LOG_LEVEL")).toBe(false);
  });
});

describe("redactText", () => {
  test("redacts KEY=value patterns", () => {
    expect(redactText("MY_API_KEY=sk-abc123")).toBe(`MY_API_KEY=${REDACTED}`);
    expect(redactText('SECRET: "hunter2"')).toBe(`SECRET: "${REDACTED}`);
    expect(redactText("DB_PASSWORD=p@ss")).toBe(`DB_PASSWORD=${REDACTED}`);
  });

  test("redacts URL credentials", () => {
    expect(redactText("postgres://admin:s3cret@db.host:5432/mydb")).toBe(
      `postgres://admin:${REDACTED}@db.host:5432/mydb`,
    );
  });

  test("leaves clean text unchanged", () => {
    const clean = "all systems operational";
    expect(redactText(clean)).toBe(clean);
  });

  test("handles multiple sensitive values", () => {
    const input = "MY_API_KEY=abc TOKEN=xyz";
    const result = redactText(input);
    expect(result).toContain(`MY_API_KEY=${REDACTED}`);
    expect(result).toContain(`TOKEN=${REDACTED}`);
    expect(result).not.toContain("abc");
    expect(result).not.toContain("xyz");
  });

  test("is safe to call multiple times (global regex reset)", () => {
    redactText("TOKEN=aaa");
    const result = redactText("TOKEN=bbb");
    expect(result).toBe(`TOKEN=${REDACTED}`);
  });
});

describe("redactEnviron", () => {
  test("redacts sensitive keys entirely", () => {
    const result = redactEnviron(["API_KEY=sk-12345", "PORT=8080"]);
    expect(result["API_KEY"]).toBe(REDACTED);
    expect(result["PORT"]).toBe("8080");
  });

  test("scans non-sensitive values for embedded secrets", () => {
    const result = redactEnviron(["DATABASE_URL=postgres://user:pass@host/db"]);
    expect(result["DATABASE_URL"]).toContain(REDACTED);
    expect(result["DATABASE_URL"]).not.toContain("pass");
  });

  test("handles entries without =", () => {
    const result = redactEnviron(["STANDALONE"]);
    expect(result["STANDALONE"]).toBe("");
  });
});

describe("redactErrorMessage", () => {
  test("redacts tokens in error messages", () => {
    const msg = "failed: SESSION_TOKEN=session-abc123 invalid";
    expect(redactErrorMessage(msg)).toContain(REDACTED);
    expect(redactErrorMessage(msg)).not.toContain("session-abc123");
  });
});
