/**
 * Redaction utilities for gateway responses and logs.
 *
 * Mirrors the patterns from daemon/internal/redact/redact.go so that
 * sensitive values (API keys, tokens, passwords, URL credentials) are
 * scrubbed before they leave the gateway in error messages or log output.
 */

export const REDACTED = "***REDACTED***";

const SENSITIVE_KEY_PARTS = [
  "API_KEY",
  "SECRET",
  "TOKEN",
  "PASSWORD",
  "CREDENTIAL",
];

/**
 * Matches inline key=value style sensitive data such as:
 *   MY_API_KEY=abc123
 *   SECRET: "xyzzy"
 */
const sensitiveTextPattern =
  /\b([A-Z0-9_]*(?:API_KEY|SECRET|TOKEN|PASSWORD|CREDENTIAL)[A-Z0-9_]*)\b(\s*[:=]\s*"?)([^,\s"'\n]+)"?/gi;

/**
 * Matches credentials embedded in URLs: scheme://user:password@host
 */
const urlCredentialPattern = /(:\/\/[^:/@\s]+):([^@\s]+)@/g;

/** Returns true if the key name looks like it holds a secret. */
export function isSensitiveKey(key: string): boolean {
  const upper = key.toUpperCase();
  return SENSITIVE_KEY_PARTS.some((part) => upper.includes(part));
}

/**
 * Redact sensitive patterns found in free-form text.
 *
 * - KEY=value / KEY: "value" pairs where KEY contains a sensitive word
 * - URL-embedded credentials (scheme://user:pass@host)
 */
export function redactText(input: string): string {
  // Reset lastIndex for global regexes
  sensitiveTextPattern.lastIndex = 0;
  urlCredentialPattern.lastIndex = 0;

  let result = input.replace(sensitiveTextPattern, `$1$2${REDACTED}`);
  result = result.replace(urlCredentialPattern, `$1:${REDACTED}@`);
  return result;
}

/**
 * Redact an environment-variable style record (key=value strings).
 * Keys whose names look sensitive get their entire value replaced;
 * other values are scanned with {@link redactText}.
 */
export function redactEnviron(environ: string[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const item of environ) {
    const eqIdx = item.indexOf("=");
    const key = (eqIdx === -1 ? item : item.slice(0, eqIdx)).trim();
    if (!key) continue;
    if (isSensitiveKey(key)) {
      out[key] = REDACTED;
    } else if (eqIdx !== -1) {
      out[key] = redactText(item.slice(eqIdx + 1));
    } else {
      out[key] = "";
    }
  }
  return out;
}

/**
 * Redact sensitive data from a gateway error message string.
 * Convenience wrapper around {@link redactText}.
 */
export function redactErrorMessage(message: string): string {
  return redactText(message);
}
