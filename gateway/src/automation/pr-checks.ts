/**
 * PR check status aggregation and parsing.
 *
 * Aggregates individual check conclusions into an overall status
 * for PR readiness evaluation.
 */

export type CheckConclusion =
  | "SUCCESS"
  | "FAILURE"
  | "NEUTRAL"
  | "CANCELLED"
  | "TIMED_OUT"
  | "ACTION_REQUIRED"
  | "SKIPPED"
  | "STALE"
  | "PENDING"
  | null;

export type CheckResult = {
  name: string;
  conclusion: string | null;
  status: string;
};

export type AggregateStatus = "green" | "pending" | "failing";

const GREEN_CONCLUSIONS: ReadonlySet<string> = new Set([
  "SUCCESS",
  "NEUTRAL",
  "SKIPPED",
]);

const PENDING_INDICATORS: ReadonlySet<string | null> = new Set([
  "PENDING",
  null,
  "",
]);

/**
 * Determine if a single check conclusion is considered green (passing).
 * Unknown conclusions are treated as non-green to be safe.
 */
export function isGreenConclusion(conclusion: string | null): boolean {
  if (conclusion === null || conclusion === undefined) return false;
  return GREEN_CONCLUSIONS.has(conclusion);
}

/**
 * Determine if a single check conclusion indicates pending status.
 */
export function isPendingConclusion(conclusion: string | null): boolean {
  return PENDING_INDICATORS.has(conclusion);
}

/**
 * Aggregate an array of check results into an overall status.
 *
 * Rules:
 * - If any check has a non-green, non-pending conclusion → "failing"
 * - If any check is pending and none are failing → "pending"
 * - If all checks are green → "green"
 * - Empty checks array → "pending"
 */
export function aggregateCheckStatus(checks: CheckResult[]): AggregateStatus {
  if (checks.length === 0) return "pending";

  let hasPending = false;

  for (const check of checks) {
    const conclusion = check.conclusion;

    if (isPendingConclusion(conclusion)) {
      hasPending = true;
      continue;
    }

    if (!isGreenConclusion(conclusion!)) {
      return "failing";
    }
  }

  return hasPending ? "pending" : "green";
}
