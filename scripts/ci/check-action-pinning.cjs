"use strict";

const fs = require("node:fs");
const path = require("node:path");

const SHA1_COMMIT_RE = /^[0-9a-f]{40}$/i;
const USES_LINE_RE = /^\s*(?:-\s*)?uses:\s*['"]?([^'"#\s]+)['"]?\s*(?:#.*)?$/;

function isAllowlistedTarget(target, allowlistedOwners) {
  if (target.startsWith("./") || target.startsWith("../") || target.startsWith("docker://")) {
    return true;
  }
  const atIndex = target.lastIndexOf("@");
  if (atIndex <= 0) {
    return false;
  }
  const action = target.slice(0, atIndex);
  const owner = action.split("/")[0].toLowerCase();
  return allowlistedOwners.has(owner);
}

function validateUsesTarget(target, allowlistedOwners) {
  if (isAllowlistedTarget(target, allowlistedOwners)) {
    return null;
  }

  const atIndex = target.lastIndexOf("@");
  if (atIndex <= 0) {
    return "missing @ref";
  }
  const ref = target.slice(atIndex + 1);
  if (!SHA1_COMMIT_RE.test(ref)) {
    return `mutable ref "${ref}" is not pinned to a commit SHA`;
  }
  return null;
}

function scanWorkflowContent(content, filePath, options = {}) {
  const allowlistedOwners = new Set(
    (options.allowlistedOwners ?? ["actions", "github"]).map((owner) => owner.toLowerCase()),
  );

  const violations = [];
  const lines = content.split(/\r?\n/);
  for (let idx = 0; idx < lines.length; idx += 1) {
    const line = lines[idx];
    const match = line.match(USES_LINE_RE);
    if (!match) {
      continue;
    }
    const target = match[1];
    const reason = validateUsesTarget(target, allowlistedOwners);
    if (!reason) {
      continue;
    }
    violations.push({
      file: filePath,
      line: idx + 1,
      target,
      reason,
    });
  }
  return violations;
}

function collectWorkflowFiles(rootDir) {
  const workflowsDir = path.join(rootDir, ".github", "workflows");
  if (!fs.existsSync(workflowsDir)) {
    return [];
  }
  return fs
    .readdirSync(workflowsDir, { withFileTypes: true })
    .filter((entry) => entry.isFile() && /\.(ya?ml)$/i.test(entry.name))
    .map((entry) => path.join(workflowsDir, entry.name))
    .sort();
}

function checkActionPinning(rootDir, options = {}) {
  const files = collectWorkflowFiles(rootDir);
  const violations = [];
  for (const filePath of files) {
    const content = fs.readFileSync(filePath, "utf8");
    violations.push(...scanWorkflowContent(content, filePath, options));
  }
  return { files, violations };
}

function formatViolations(violations) {
  return violations
    .map((violation) => `${violation.file}:${violation.line} uses ${violation.target} (${violation.reason})`)
    .join("\n");
}

function runCli() {
  const rootDir = process.argv[2] ? path.resolve(process.argv[2]) : process.cwd();
  const { files, violations } = checkActionPinning(rootDir);
  if (violations.length > 0) {
    console.error("Found mutable action references:");
    console.error(formatViolations(violations));
    process.exitCode = 1;
    return;
  }
  console.log(`Action pinning guard passed (${files.length} workflow files scanned).`);
}

if (require.main === module) {
  runCli();
}

module.exports = {
  checkActionPinning,
  collectWorkflowFiles,
  formatViolations,
  scanWorkflowContent,
  validateUsesTarget,
};
