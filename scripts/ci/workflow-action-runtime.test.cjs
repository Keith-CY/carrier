"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const repoRoot = path.resolve(__dirname, "..", "..");
const workflowDir = path.join(repoRoot, ".github", "workflows");

const deprecatedRefs = [
  "actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02",
  "oven-sh/setup-bun@3d267786b128fe76c2f16a390aa2448b815359f3",
  "oven-sh/setup-bun@735343b667d3e6f658f44d0eca948eb6282f2b76",
];

function listWorkflowFiles(dir) {
  return fs.readdirSync(dir)
    .filter((name) => name.endsWith(".yml") || name.endsWith(".yaml"))
    .map((name) => path.join(dir, name));
}

test("workflow pins avoid deprecated node20 action runtimes", () => {
  const offenders = [];

  for (const file of listWorkflowFiles(workflowDir)) {
    const content = fs.readFileSync(file, "utf8");
    for (const ref of deprecatedRefs) {
      if (content.includes(ref)) {
        offenders.push(`${path.relative(repoRoot, file)} -> ${ref}`);
      }
    }
  }

  assert.deepEqual(offenders, []);
});
