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
      passed: 86,
      violations: 0,
      legacyOversized: 0,
      maxLines: 500,
      detailsMarkdown: "",
      fail: false,
    },
  };
}

test("readability summary reports within-limit, violations, and legacy warnings", () => {
  const comment = buildMetricsHarnessComment(buildBaseInput());

  assert.match(comment, /### Code Readability \(hard gate on new threshold crossings\) ✅/);
  assert.match(comment, /\| Within limit \| 86\/86 \| ≤ 500 lines \| ✅ \| files at or under target \|/);
  assert.match(comment, /\| Hard violations \| 0 \| hard \| ✅ \| new files or threshold crossings only \|/);
  assert.match(comment, /\| Legacy oversized warnings \| 0 \| soft \| ✅ \| pre-existing >500-line files touched by the PR \|/);
  assert.doesNotMatch(comment, /<details><summary>Per-file breakdown<\/summary>/);
});

test("readability section warns on legacy oversized files without hard-failing", () => {
  const input = buildBaseInput();
  input.readability.checked = 3;
  input.readability.passed = 2;
  input.readability.legacyOversized = 1;
  input.readability.detailsMarkdown =
    "| `baseagent/runtime_structured_loop_test.go` | 1554 | 1554 | soft | ⚠️ | legacy oversized file |\n";

  const comment = buildMetricsHarnessComment(input);

  assert.match(comment, /### Code Readability \(hard gate on new threshold crossings\) ⚠️/);
  assert.match(comment, /\| Within limit \| 2\/3 \| ≤ 500 lines \| ℹ️ \| files at or under target \|/);
  assert.match(comment, /\| Hard violations \| 0 \| hard \| ✅ \| new files or threshold crossings only \|/);
  assert.match(comment, /\| Legacy oversized warnings \| 1 \| soft \| ⚠️ \| pre-existing >500-line files touched by the PR \|/);
  assert.match(comment, /<details><summary>Per-file breakdown<\/summary>/);
  assert.match(comment, /\| `baseagent\/runtime_structured_loop_test\.go` \| 1554 \| 1554 \| soft \| ⚠️ \| legacy oversized file \|/);
});

test("readability section hard-fails only on new threshold crossings", () => {
  const input = buildBaseInput();
  input.readability.checked = 3;
  input.readability.passed = 1;
  input.readability.legacyOversized = 1;
  input.readability.violations = 1;
  input.readability.fail = true;
  input.readability.detailsMarkdown = [
    "| `baseagent/runtime_structured_loop_test.go` | 1554 | 1554 | soft | ⚠️ | legacy oversized file |",
    "| `gateway/too-large.go` | 498 | 701 | hard | ❌ | new or newly oversized file |",
  ].join("\n");

  const comment = buildMetricsHarnessComment(input);

  assert.match(comment, /### Code Readability \(hard gate on new threshold crossings\) ❌/);
  assert.match(comment, /\| Within limit \| 1\/3 \| ≤ 500 lines \| ℹ️ \| files at or under target \|/);
  assert.match(comment, /\| Hard violations \| 1 \| hard \| ❌ \| new files or threshold crossings only \|/);
  assert.match(comment, /\| `gateway\/too-large\.go` \| 498 \| 701 \| hard \| ❌ \| new or newly oversized file \|/);
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

test("env-like 0/1 flags still drive hard and soft gate status correctly", () => {
  const input = buildBaseInput();
  input.bundle.hardFail = "1";
  input.commit.fail = "1";
  input.readability.fail = "0";
  input.perf.remoteSuiteOk = "0";
  input.perf.gatewaySuiteOk = "1";

  const comment = buildMetricsHarnessComment(input);

  assert.match(comment, /\*\*Status\*\*: ❌ Hard gates failed/);
  assert.match(comment, /### Commit Size \(soft gate\) ❌/);
  assert.match(comment, /### WebUI Bundle Size \(hard gate\) ❌/);
  assert.match(comment, /### Code Readability \(hard gate on new threshold crossings\) ✅/);
  assert.match(comment, /### Perf Probes \(soft gate\) ⚠️/);
});
