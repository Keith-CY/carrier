import { describe, expect, test } from "bun:test";
import {
  aggregateCheckStatus,
  isGreenConclusion,
  isPendingConclusion,
  type CheckResult,
} from "./pr-checks";

function check(name: string, conclusion: string | null): CheckResult {
  return { name, conclusion, status: conclusion === null ? "IN_PROGRESS" : "COMPLETED" };
}

describe("isGreenConclusion", () => {
  test("SUCCESS is green", () => {
    expect(isGreenConclusion("SUCCESS")).toBe(true);
  });
  test("NEUTRAL is green", () => {
    expect(isGreenConclusion("NEUTRAL")).toBe(true);
  });
  test("SKIPPED is green", () => {
    expect(isGreenConclusion("SKIPPED")).toBe(true);
  });
  test("FAILURE is not green", () => {
    expect(isGreenConclusion("FAILURE")).toBe(false);
  });
  test("null is not green", () => {
    expect(isGreenConclusion(null)).toBe(false);
  });
  test("unknown conclusion is not green", () => {
    expect(isGreenConclusion("STARTUP_FAILURE")).toBe(false);
  });
  test("empty string is not green", () => {
    expect(isGreenConclusion("")).toBe(false);
  });
  test("arbitrary unknown string is not green", () => {
    expect(isGreenConclusion("SOME_NEW_STATUS")).toBe(false);
  });
});

describe("isPendingConclusion", () => {
  test("PENDING is pending", () => {
    expect(isPendingConclusion("PENDING")).toBe(true);
  });
  test("null is pending", () => {
    expect(isPendingConclusion(null)).toBe(true);
  });
  test("empty string is pending", () => {
    expect(isPendingConclusion("")).toBe(true);
  });
  test("SUCCESS is not pending", () => {
    expect(isPendingConclusion("SUCCESS")).toBe(false);
  });
});

describe("aggregateCheckStatus", () => {
  test("empty checks returns pending", () => {
    expect(aggregateCheckStatus([])).toBe("pending");
  });

  test("all SUCCESS returns green", () => {
    expect(aggregateCheckStatus([
      check("ci-1", "SUCCESS"),
      check("ci-2", "SUCCESS"),
    ])).toBe("green");
  });

  test("SUCCESS + NEUTRAL + SKIPPED returns green", () => {
    expect(aggregateCheckStatus([
      check("ci-1", "SUCCESS"),
      check("ci-2", "NEUTRAL"),
      check("ci-3", "SKIPPED"),
    ])).toBe("green");
  });

  test("any FAILURE returns failing", () => {
    expect(aggregateCheckStatus([
      check("ci-1", "SUCCESS"),
      check("ci-2", "FAILURE"),
    ])).toBe("failing");
  });

  test("PENDING with SUCCESS returns pending", () => {
    expect(aggregateCheckStatus([
      check("ci-1", "SUCCESS"),
      check("ci-2", null),
    ])).toBe("pending");
  });

  // --- Unknown conclusion handling ---
  test("unknown conclusion is treated as failing", () => {
    expect(aggregateCheckStatus([
      check("ci-1", "SUCCESS"),
      check("ci-2", "STARTUP_FAILURE"),
    ])).toBe("failing");
  });

  test("unknown conclusion alone is failing", () => {
    expect(aggregateCheckStatus([
      check("ci-1", "SOME_NEW_CONCLUSION"),
    ])).toBe("failing");
  });

  test("mixed SUCCESS + unknown conclusion is failing", () => {
    expect(aggregateCheckStatus([
      check("ci-1", "SUCCESS"),
      check("ci-2", "NEUTRAL"),
      check("ci-3", "UNEXPECTED_VALUE"),
    ])).toBe("failing");
  });

  test("unknown conclusion with pending checks is failing (fail takes priority)", () => {
    expect(aggregateCheckStatus([
      check("ci-1", null),
      check("ci-2", "WEIRD_CONCLUSION"),
    ])).toBe("failing");
  });

  test("CANCELLED is treated as failing", () => {
    expect(aggregateCheckStatus([
      check("ci-1", "SUCCESS"),
      check("ci-2", "CANCELLED"),
    ])).toBe("failing");
  });

  test("TIMED_OUT is treated as failing", () => {
    expect(aggregateCheckStatus([
      check("ci-1", "TIMED_OUT"),
    ])).toBe("failing");
  });

  test("ACTION_REQUIRED is treated as failing", () => {
    expect(aggregateCheckStatus([
      check("ci-1", "ACTION_REQUIRED"),
    ])).toBe("failing");
  });

  test("STALE is treated as failing", () => {
    expect(aggregateCheckStatus([
      check("ci-1", "STALE"),
    ])).toBe("failing");
  });

  test("does not crash on large variety of unknown conclusions", () => {
    const unknowns = ["FOO", "BAR_BAZ", "123", "success", "True", "UNKNOWN"];
    for (const u of unknowns) {
      const result = aggregateCheckStatus([check("ci", u)]);
      expect(result).toBe("failing");
    }
  });
});
