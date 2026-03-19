"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");

const { buildMetricsHarnessComment } = require("./metrics-harness-comment.cjs");

function buildBaseInput() {
  return {
    headRef: "fix/metrics-report-comment",
    bundle: {
      hardFail: false,
      rawKb: 939,
      gzipKb: 288,
      initialGzipKb: 139,
      gzipLimitKb: 350,
      initialLimitKb: 180,
    },
    commit: {
      total: 1,
      passed: 1,
      maxLines: 500,
      maxSeen: 81,
      detailsMarkdown: "| `abc1234` | 81 | ≤ 500 | ✅ | subject |\n",
      fail: false,
    },
    perf: {
      buildMs: 25033,
      binarySizeMb: "24.2",
      cmdRuns: 9,
      cmdFailures: 0,
      overallP95Ms: 5,
      cmdLimitMs: 800,
      remoteSuiteOk: true,
      remoteSuiteMs: 7583,
      gatewaySuiteOk: true,
      gatewaySuiteMs: 15595,
      sshReady: false,
      sshCmdRuns: 0,
      sshCmdFailures: 0,
      sshOverallP95Ms: 0,
      sshCmdLimitMs: 2500,
      sshBatchP50Ms: 0,
      cmdDetailsMarkdown: "| `carrier --help` | 3 | 0 | 4 ms | 4 ms | 4 ms | ✅ |\n",
      sshDetailsMarkdown: "",
    },
    readability: {
      checked: 86,
      violations: 0,
      maxLines: 500,
      failuresMarkdown: "",
      fail: false,
    },
  };
}

test("readability summary omits passing file list and reports passed/total", () => {
  const comment = buildMetricsHarnessComment(buildBaseInput());

  assert.match(comment, /\| Within limit \| 86\/86 \| ≤ 500 lines \| ✅ \|/);
  assert.match(comment, /\| Violations \| 0 \| must be 0 \| ✅ \|/);
  assert.doesNotMatch(comment, /\| File \| Lines \| Limit \| Status \|/);
  assert.doesNotMatch(comment, /baseagent\/controlplane_providers\.go/);
});

test("readability section lists only failing files when violations exist", () => {
  const input = buildBaseInput();
  input.readability.checked = 3;
  input.readability.violations = 1;
  input.readability.fail = true;
  input.readability.failuresMarkdown = "| `gateway/too-large.go` | 701 | ≤ 500 | ❌ |\n";

  const comment = buildMetricsHarnessComment(input);

  assert.match(comment, /\| Within limit \| 2\/3 \| ≤ 500 lines \| ❌ \|/);
  assert.match(comment, /<details><summary>Files over limit<\/summary>/);
  assert.match(comment, /\| `gateway\/too-large\.go` \| 701 \| ≤ 500 \| ❌ \|/);
  assert.doesNotMatch(comment, /baseagent\/small\.go/);
});

test("perf probes treat unavailable SSH loopback as skipped info instead of warning", () => {
  const comment = buildMetricsHarnessComment(buildBaseInput());

  assert.match(comment, /### Perf Probes \(soft gate\) ✅/);
  assert.match(comment, /\| remote SSH available \| false \| when available \| ℹ️ \|/);
  assert.match(comment, /\| remote SSH probe runs \| 0 \| failures = 0 when available \| ℹ️ \|/);
  assert.match(comment, /\| _\(ssh probe skipped: loopback SSH unavailable on runner\)_ \| 0 \| 0 \| 0 ms \| 0 ms \| 0 ms \| ℹ️ \|/);
});

test("perf probes still warn when measured SSH timings exceed limits or fail", () => {
  const input = buildBaseInput();
  input.perf.sshReady = true;
  input.perf.sshCmdRuns = 9;
  input.perf.sshCmdFailures = 1;
  input.perf.sshOverallP95Ms = 3100;
  input.perf.sshBatchP50Ms = 1600;
  input.perf.sshDetailsMarkdown = "| `ssh:help` | 3 | 1 | 900 ms | 3100 ms | 3200 ms | ⚠️ |\n";

  const comment = buildMetricsHarnessComment(input);

  assert.match(comment, /### Perf Probes \(soft gate\) ⚠️/);
  assert.match(comment, /\| remote SSH available \| true \| when available \| ✅ \|/);
  assert.match(comment, /\| remote SSH overall P95 \| 3100 ms \| ≤ 2500 ms \(soft, when available\) \| ⚠️ \|/);
});

test("comment builder tolerates missing nested inputs", () => {
  const comment = buildMetricsHarnessComment();

  assert.match(comment, /## 📏 Carrier Metrics Harness Report/);
  assert.match(comment, /\| Commits checked \| 0 \| — \| — \|/);
  assert.match(comment, /\| JS bundle \(raw\) \| 0 KB \| — \| — \|/);
  assert.match(comment, /\| Source files checked \| 0 \| — \| — \|/);
});
