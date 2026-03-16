"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const repoRoot = path.resolve(__dirname, "..", "..");
const workflowPath = path.join(repoRoot, ".github", "workflows", "ci.yml");

function extractStepScript(workflow, stepName, nextStepName) {
  const pattern = new RegExp(
    String.raw`- name: ${stepName}\n\s+run: \|\n([\s\S]*?)\n\s+- name: ${nextStepName}`,
  );
  const match = workflow.match(pattern);
  assert.ok(match, `expected to find workflow step ${stepName}`);
  return match[1];
}

test("coverage comment writes WebUI head coverage where the summary expects it", () => {
  const workflow = fs.readFileSync(workflowPath, "utf8");
  const headCoverageScript = extractStepScript(
    workflow,
    "Collect head branch coverage",
    "Collect base branch coverage",
  );

  assert.match(
    workflow,
    /"head_profile": "coverage-webui\.out"/,
    "summary should read the WebUI head coverage profile from the repository root",
  );

  const writesRootProfile =
    /\(\s*cd\s+webui\s+&&\s+go\s+test\b[\s\S]*?-coverprofile=\.\.\/coverage-webui\.out\s*\)/.test(headCoverageScript) ||
    /cp\s+webui\/coverage-webui\.out\s+coverage-webui\.out\b/.test(headCoverageScript);

  assert.ok(
    writesRootProfile,
    "Collect head branch coverage must place the WebUI profile at coverage-webui.out in the repository root",
  );
});
