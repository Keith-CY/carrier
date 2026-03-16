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
    headCoverageScript.includes("(cd webui && go test ./... -coverprofile=../coverage-webui.out)") ||
    headCoverageScript.includes("cp webui/coverage-webui.out coverage-webui.out");

  assert.equal(
    writesRootProfile,
    true,
    "Collect head branch coverage must place the WebUI profile at coverage-webui.out in the repository root",
  );
});
