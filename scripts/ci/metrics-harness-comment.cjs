"use strict";

const fs = require("node:fs");

const COMMENT_MARKER = "<!-- carrier-pr-metrics-harness -->";

function asNumber(value, fallback = 0) {
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : fallback;
}

function asBool(value) {
  if (value === true || value === 1) {
    return true;
  }

  if (value === false || value === 0 || value == null) {
    return false;
  }

  const normalized = String(value).trim().toLowerCase();
  return normalized === "true" || normalized === "1";
}

function readTextFile(filePath) {
  if (!filePath || !fs.existsSync(filePath)) {
    return "";
  }
  return fs.readFileSync(filePath, "utf8");
}

function buildReadabilityBlock(readability, icon) {
  const checked = asNumber(readability.checked);
  const violations = asNumber(readability.violations);
  const maxLines = asNumber(readability.maxLines, 500);
  const withinLimit = Number.isFinite(Number(readability.passed))
    ? Math.max(0, asNumber(readability.passed))
    : Math.max(0, checked - violations);
  const failuresMarkdown = String(readability.failuresMarkdown || "").trim();

  let block = [
    `### Code Readability (hard gate: changed source files) ${icon}`,
    "",
    "| Metric | Value | Limit | Status |",
    "|--------|-------|-------|--------|",
    `| Source files checked | ${checked} | — | — |`,
    `| Within limit | ${withinLimit}/${checked} | ≤ ${maxLines} lines | ${violations === 0 ? "✅" : "❌"} |`,
    `| Violations | ${violations} | must be 0 | ${violations === 0 ? "✅" : "❌"} |`,
  ].join("\n");

  if (failuresMarkdown) {
    block += [
      "",
      "<details><summary>Files over limit</summary>",
      "",
      "| File | Lines | Limit | Status |",
      "|------|-------|-------|--------|",
      failuresMarkdown,
      "",
      "</details>",
    ].join("\n");
  }

  return block;
}

function buildPerfSummary(perf) {
  const sshMeasured = asBool(perf.sshReady);
  const cmdFailures = asNumber(perf.cmdFailures);
  const remoteSuiteOk = asBool(perf.remoteSuiteOk);
  const gatewaySuiteOk = asBool(perf.gatewaySuiteOk);
  const sshCmdFailures = asNumber(perf.sshCmdFailures);
  const sshOverallP95Ms = asNumber(perf.sshOverallP95Ms);
  const sshCmdLimitMs = asNumber(perf.sshCmdLimitMs, 2500);

  const warn = (
    cmdFailures > 0 ||
    !remoteSuiteOk ||
    !gatewaySuiteOk ||
    (sshMeasured && (sshCmdFailures > 0 || sshOverallP95Ms > sshCmdLimitMs))
  );

  return {
    icon: warn ? "⚠️" : "✅",
    sshMeasured,
  };
}

function buildPerfBlock(perf, icon) {
  const sshMeasured = asBool(perf.sshReady);
  const sshCmdFailures = asNumber(perf.sshCmdFailures);
  const sshCmdLimitMs = asNumber(perf.sshCmdLimitMs, 2500);
  const sshOverallP95Ms = asNumber(perf.sshOverallP95Ms);
  const sshBatchP50Ms = asNumber(perf.sshBatchP50Ms);

  const sshAvailabilityStatus = sshMeasured ? "✅" : "ℹ️";
  const sshRunStatus = !sshMeasured ? "ℹ️" : (sshCmdFailures === 0 ? "✅" : "⚠️");
  const sshP95Status = !sshMeasured ? "ℹ️" : (sshOverallP95Ms <= sshCmdLimitMs ? "✅" : "⚠️");
  const sshBatchStatus = "ℹ️";
  const sshDetailsMarkdown = String(perf.sshDetailsMarkdown || "").trim() || (
    sshMeasured
      ? "| _(no ssh detail rows recorded)_ | 0 | 0 | 0 ms | 0 ms | 0 ms | ℹ️ |"
      : "| _(ssh probe skipped: loopback SSH unavailable on runner)_ | 0 | 0 | 0 ms | 0 ms | 0 ms | ℹ️ |"
  );

  return [
    `### Perf Probes (soft gate) ${icon}`,
    "",
    "| Metric | Value | Target | Status |",
    "|--------|-------|--------|--------|",
    `| carrier build time | ${asNumber(perf.buildMs)} ms | trend ↓ | ℹ️ |`,
    `| carrier binary size | ${perf.binarySizeMb} MB | trend ↓ | ℹ️ |`,
    `| CLI probe runs | ${asNumber(perf.cmdRuns)} | failures = 0 | ${asNumber(perf.cmdFailures) === 0 ? "✅" : "⚠️"} |`,
    `| CLI overall P95 | ${asNumber(perf.overallP95Ms)} ms | ≤ ${asNumber(perf.cmdLimitMs, 800)} ms (soft) | ${asNumber(perf.overallP95Ms) <= asNumber(perf.cmdLimitMs, 800) ? "✅" : "⚠️"} |`,
    `| cmd/carrier remote-path suite | ${asNumber(perf.remoteSuiteMs)} ms | pass | ${asBool(perf.remoteSuiteOk) ? "✅" : "⚠️"} |`,
    `| gateway remote-metrics suite | ${asNumber(perf.gatewaySuiteMs)} ms | pass | ${asBool(perf.gatewaySuiteOk) ? "✅" : "⚠️"} |`,
    `| remote SSH available | ${sshMeasured} | when available | ${sshAvailabilityStatus} |`,
    `| remote SSH probe runs | ${asNumber(perf.sshCmdRuns)} | failures = 0 when available | ${sshRunStatus} |`,
    `| remote SSH overall P95 | ${sshOverallP95Ms} ms | ≤ ${sshCmdLimitMs} ms (soft, when available) | ${sshP95Status} |`,
    `| remote SSH batch P50 | ${sshBatchP50Ms} ms | trend ↓ (when available) | ${sshBatchStatus} |`,
    "",
    "<details><summary>CLI command timings</summary>",
    "",
    "| Command | Runs | Failures | P50 | P95 | Max | Status |",
    "|---------|------|----------|-----|-----|-----|--------|",
    String(perf.cmdDetailsMarkdown || "").trim(),
    "",
    "</details>",
    "",
    "<details><summary>Remote SSH timings</summary>",
    "",
    "| Command | Runs | Failures | P50 | P95 | Max | Status |",
    "|---------|------|----------|-----|-----|-----|--------|",
    sshDetailsMarkdown,
    "",
    "</details>",
  ].join("\n");
}

function buildMetricsHarnessComment(input) {
  const {
    bundle = {},
    commit = {},
    perf = {},
    readability = {},
    headRef = "",
  } = input || {};

  const bundleHardFail = asBool(bundle.hardFail);
  const readabilityFail = asBool(readability.fail);
  const hardFail = bundleHardFail || readabilityFail;
  const overall = hardFail ? "❌ Hard gates failed" : "✅ All hard gates passed";

  const commitIcon = asBool(commit.fail) ? "❌" : "✅";
  const bundleIcon = bundleHardFail ? "❌" : "✅";
  const readabilityIcon = readabilityFail ? "❌" : "✅";
  const perfSummary = buildPerfSummary(perf);

  return [
    COMMENT_MARKER,
    "## 📏 Carrier Metrics Harness Report",
    "",
    `**Status**: ${overall}`,
    "",
    `### Commit Size (soft gate) ${commitIcon}`,
    "",
    "| Metric | Value | Limit | Status |",
    "|--------|-------|-------|--------|",
    `| Commits checked | ${asNumber(commit.total)} | — | — |`,
    `| All within limit | ${asNumber(commit.passed)}/${asNumber(commit.total)} | ≤ ${asNumber(commit.maxLines, 500)} lines | ${commitIcon} |`,
    `| Largest commit | ${asNumber(commit.maxSeen)} lines | ≤ ${asNumber(commit.maxLines, 500)} | ${asNumber(commit.maxSeen) <= asNumber(commit.maxLines, 500) ? "✅" : "❌"} |`,
    "",
    "<details><summary>Per-commit breakdown</summary>",
    "",
    "| Commit | Lines changed | Limit | Status | Subject |",
    "|--------|---------------|-------|--------|---------|",
    String(commit.detailsMarkdown || "").trim(),
    "",
    "</details>",
    "",
    `### WebUI Bundle Size (hard gate) ${bundleIcon}`,
    "",
    "| Metric | Value | Limit | Status |",
    "|--------|-------|-------|--------|",
    `| JS bundle (raw) | ${asNumber(bundle.rawKb)} KB | — | — |`,
    `| JS bundle (gzip) | ${asNumber(bundle.gzipKb)} KB | ≤ ${asNumber(bundle.gzipLimitKb)} KB | ${asNumber(bundle.gzipKb) <= asNumber(bundle.gzipLimitKb) ? "✅" : "❌"} |`,
    `| JS initial load (gzip) | ${asNumber(bundle.initialGzipKb)} KB | ≤ ${asNumber(bundle.initialLimitKb)} KB | ${asNumber(bundle.initialGzipKb) <= asNumber(bundle.initialLimitKb) ? "✅" : "❌"} |`,
    "",
    buildPerfBlock(perf, perfSummary.icon),
    "",
    buildReadabilityBlock(readability, readabilityIcon),
    "",
    "---",
    `> 📊 Metrics spec: [\`docs/architecture/metrics.md\`](../blob/${headRef}/docs/architecture/metrics.md)`,
    "",
  ].join("\n");
}

function loadInputFromEnv(env = process.env) {
  return {
    headRef: env.METRICS_HEAD_REF,
    bundle: {
      hardFail: env.METRICS_BUNDLE_HARD_FAIL,
      rawKb: env.METRICS_BUNDLE_RAW_KB,
      gzipKb: env.METRICS_BUNDLE_GZIP_KB,
      initialGzipKb: env.METRICS_BUNDLE_INITIAL_GZIP_KB,
      gzipLimitKb: env.METRICS_BUNDLE_GZIP_LIMIT_KB,
      initialLimitKb: env.METRICS_BUNDLE_INITIAL_LIMIT_KB,
    },
    commit: {
      total: env.METRICS_COMMIT_TOTAL,
      passed: env.METRICS_COMMIT_PASSED,
      maxLines: env.METRICS_COMMIT_MAX_LINES,
      maxSeen: env.METRICS_COMMIT_MAX_SEEN,
      detailsMarkdown: readTextFile(env.METRICS_COMMIT_DETAILS_PATH),
      fail: env.METRICS_COMMIT_FAIL,
    },
    perf: {
      buildMs: env.METRICS_PERF_BUILD_MS,
      binarySizeMb: env.METRICS_PERF_BINARY_SIZE_MB,
      cmdRuns: env.METRICS_PERF_CMD_RUNS,
      cmdFailures: env.METRICS_PERF_CMD_FAILURES,
      overallP95Ms: env.METRICS_PERF_OVERALL_P95_MS,
      cmdLimitMs: env.METRICS_PERF_CMD_LIMIT_MS,
      remoteSuiteOk: env.METRICS_PERF_REMOTE_SUITE_OK,
      remoteSuiteMs: env.METRICS_PERF_REMOTE_SUITE_MS,
      gatewaySuiteOk: env.METRICS_PERF_GATEWAY_SUITE_OK,
      gatewaySuiteMs: env.METRICS_PERF_GATEWAY_SUITE_MS,
      sshReady: env.METRICS_PERF_SSH_READY,
      sshCmdRuns: env.METRICS_PERF_SSH_CMD_RUNS,
      sshCmdFailures: env.METRICS_PERF_SSH_CMD_FAILURES,
      sshOverallP95Ms: env.METRICS_PERF_SSH_OVERALL_P95_MS,
      sshCmdLimitMs: env.METRICS_PERF_SSH_CMD_LIMIT_MS,
      sshBatchP50Ms: env.METRICS_PERF_SSH_BATCH_P50_MS,
      cmdDetailsMarkdown: readTextFile(env.METRICS_PERF_CMD_DETAILS_PATH),
      sshDetailsMarkdown: readTextFile(env.METRICS_PERF_SSH_DETAILS_PATH),
    },
    readability: {
      checked: env.METRICS_READABILITY_CHECKED,
      passed: env.METRICS_READABILITY_PASSED,
      violations: env.METRICS_READABILITY_VIOLATIONS,
      maxLines: env.METRICS_READABILITY_MAX_LINES,
      failuresMarkdown: readTextFile(env.METRICS_READABILITY_FAILURES_PATH),
      fail: env.METRICS_READABILITY_FAIL,
    },
  };
}

if (require.main === module) {
  process.stdout.write(buildMetricsHarnessComment(loadInputFromEnv()));
}

module.exports = {
  buildMetricsHarnessComment,
  loadInputFromEnv,
};
