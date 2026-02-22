"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");

const {
  DELTA_COMMENT_MARKER,
  buildAuditDeltaComment,
  computeCleanRunGate,
  computeDocsSupersededTrend,
  computeMetricDrift,
  computePrWatchdog,
  extractStateFromBody,
  isAutomationAuditPr,
  upsertStateMarker,
} = require("./hourly-kanban-metrics.cjs");

test("state marker roundtrip preserves metrics and streak", () => {
  const original = {
    metrics: {
      docs_todo: 1,
      core_todo: 2,
      docs_superseded: 3,
      test_files: 4,
    },
    cleanRunStreak: 5,
  };

  const body = upsertStateMarker("# header", original);
  const parsed = extractStateFromBody(body);

  assert.equal(parsed.metrics.docs_todo, 1);
  assert.equal(parsed.metrics.core_todo, 2);
  assert.equal(parsed.metrics.docs_superseded, 3);
  assert.equal(parsed.metrics.test_files, 4);
  assert.equal(parsed.cleanRunStreak, 5);
});

test("computeCleanRunGate increments and resets streak", () => {
  const prev = { cleanRunStreak: 2 };
  const inc = computeCleanRunGate(prev, { docs_todo: 0, core_todo: 0 }, 3);
  assert.equal(inc.cleanRun, true);
  assert.equal(inc.cleanRunStreak, 3);
  assert.equal(inc.readyForReview, true);

  const reset = computeCleanRunGate(prev, { docs_todo: 1, core_todo: 0 }, 3);
  assert.equal(reset.cleanRun, false);
  assert.equal(reset.cleanRunStreak, 0);
  assert.equal(reset.readyForReview, false);
});

test("computeMetricDrift emits alert only when threshold crossed", () => {
  const calm = computeMetricDrift(10, 12, 5);
  assert.equal(calm.hasPrevious, true);
  assert.equal(calm.delta, 2);
  assert.equal(calm.alert, false);

  const spike = computeMetricDrift(10, 20, 5);
  assert.equal(spike.alert, true);
});

test("computeDocsSupersededTrend tracks regression and keeps window", () => {
  const prev = {
    docsSupersededHistory: [
      { ts: "2026-02-22T00:00:00Z", value: 3 },
      { ts: "2026-02-22T01:00:00Z", value: 4 },
    ],
  };

  const trend = computeDocsSupersededTrend(prev, 5, "2026-02-22T02:00:00Z", 2);
  assert.equal(trend.previousValue, 4);
  assert.equal(trend.delta, 1);
  assert.equal(trend.regression, true);
  assert.equal(trend.history.length, 2);
  assert.equal(trend.history[1].value, 5);
});

test("computePrWatchdog increases unchanged runs for identical fingerprint", () => {
  const now = new Date("2026-02-22T08:00:00Z");
  const openPrs = [
    {
      number: 1273,
      title: "docs: refresh current architecture audit snapshot",
      url: "https://example.invalid/pr/1273",
      headRef: "auto/docs-audit-snapshot-20260222080000",
      headSha: "abc123",
      changedFiles: 2,
      additions: 20,
      deletions: 10,
      createdAt: "2026-02-22T00:00:00Z",
      updatedAt: "2026-02-22T07:00:00Z",
    },
  ];

  const prevState = {
    prWatchdog: {
      "1273": {
        fingerprint: "1273:abc123:2:20:10",
        unchangedRuns: 5,
      },
    },
  };

  const watchdog = computePrWatchdog(openPrs, prevState, { staleThresholdRuns: 6, now });
  assert.equal(watchdog.entries.length, 1);
  assert.equal(watchdog.entries[0].unchangedRuns, 6);
  assert.equal(watchdog.entries[0].stale, true);
  assert.equal(watchdog.staleEntries.length, 1);
});

test("isAutomationAuditPr matches auto branch and audit title", () => {
  assert.equal(
    isAutomationAuditPr({ headRef: "auto/docs-audit-snapshot-20260222", title: "whatever" }),
    true,
  );
  assert.equal(
    isAutomationAuditPr({ headRef: "feature/x", title: "docs: refresh architecture audit snapshot" }),
    true,
  );
  assert.equal(
    isAutomationAuditPr({ headRef: "feature/x", title: "feat: add endpoint" }),
    false,
  );
});

test("buildAuditDeltaComment includes marker and key metrics", () => {
  const comment = buildAuditDeltaComment({
    generatedAt: "2026-02-22T08:00:00Z",
    metrics: { docs_todo: 0, core_todo: 0, docs_superseded: 4, test_files: 90 },
    drifts: {
      docsTodo: { hasPrevious: true, delta: 0 },
      coreTodo: { hasPrevious: true, delta: 0 },
      testFiles: { hasPrevious: true, delta: 1 },
    },
    docsSupersededTrend: { previousValue: 4, delta: 0 },
    mergeGate: { cleanRunStreak: 3, requiredRuns: 3, readyForReview: true },
    watchdogEntry: { number: 1273, unchangedRuns: 6, openHours: 8, lastUpdatedHours: 1 },
  });

  assert.ok(comment.includes(DELTA_COMMENT_MARKER));
  assert.ok(comment.includes("docs_todo: 0"));
  assert.ok(comment.includes("ready for final review: yes"));
  assert.ok(comment.includes("PR #1273"));
});
