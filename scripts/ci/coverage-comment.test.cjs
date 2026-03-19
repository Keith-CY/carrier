"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const repoRoot = path.resolve(__dirname, "..", "..");
const workflowPath = path.join(repoRoot, ".github", "workflows", "ci.yml");

function extractStepScript(workflow, stepName, nextStepName) {
  const pattern = new RegExp(
    String.raw`- name: ${stepName}\n(?:\s+.*\n)*?\s+run: \|\n([\s\S]*?)\n\s+- name: ${nextStepName}`,
  );
  const match = workflow.match(pattern);
  assert.ok(match, `expected to find workflow step ${stepName}`);
  return match[1];
}

test("coverage comment collects nested core module profiles in the repository root", () => {
  const workflow = fs.readFileSync(workflowPath, "utf8");
  const headCoverageScript = extractStepScript(
    workflow,
    "Collect head branch coverage",
    "Collect base branch coverage",
  );

  for (const moduleName of ["baseagent", "gateway", "shared", "profilesync", "codeagent", "webui"]) {
    assert.match(
      headCoverageScript,
      new RegExp(String.raw`\(\s*cd\s+${moduleName}\s+&&\s+go\s+test\b[\s\S]*?-coverprofile=\.\./coverage-${moduleName}\.out\s*\)`),
      `Collect head branch coverage must place ${moduleName} coverage at coverage-${moduleName}.out in the repository root`,
    );
  }
});

test("coverage comment summary uses the shared node script", () => {
  const workflow = fs.readFileSync(workflowPath, "utf8");
  const buildSummaryScript = extractStepScript(
    workflow,
    "Build markdown summary",
    "Create or update PR comment",
  );

  assert.match(
    buildSummaryScript,
    /node\s+scripts\/ci\/coverage-comment\.cjs\b/,
    "Build markdown summary should delegate to scripts/ci/coverage-comment.cjs",
  );
});
