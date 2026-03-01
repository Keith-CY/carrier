"use strict";

const fs = require("node:fs");
const path = require("node:path");

const COMMENT_MARKER = "<!-- carrier-pr-test-packages -->";

function collectMetadataFiles(dir, out) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      collectMetadataFiles(fullPath, out);
      continue;
    }
    if (entry.isFile() && entry.name.endsWith(".json")) {
      out.push(fullPath);
    }
  }
}

function listMetadataFiles(metadataDir) {
  if (!fs.existsSync(metadataDir)) {
    return [];
  }

  const files = [];
  collectMetadataFiles(metadataDir, files);
  return files.sort();
}

function parseMetadataFile(filePath) {
  const raw = JSON.parse(fs.readFileSync(filePath, "utf8"));
  const label = String(raw.label || "").trim();
  const variant = String(raw.variant || "").trim();
  const packageFile = String(raw.package_file || "").trim();
  const artifactId = String(raw.artifact_id || "").trim();

  if (!label) {
    throw new Error(`Missing "label" in ${filePath}`);
  }
  if (!packageFile) {
    throw new Error(`Missing "package_file" in ${filePath}`);
  }
  // Release workflows currently publish .zip artifacts only.
  if (!packageFile.endsWith(".zip")) {
    throw new Error(`Expected "package_file" to end with .zip in ${filePath}, got "${packageFile}"`);
  }
  if (!artifactId || !/^\d+$/.test(artifactId)) {
    throw new Error(`Invalid "artifact_id" in ${filePath}: "${artifactId}"`);
  }

  return {
    label,
    variant,
    packageFile,
    artifactId,
  };
}

function loadPackageMetadata(metadataDir) {
  const files = listMetadataFiles(metadataDir);
  return files.map((filePath) => parseMetadataFile(filePath));
}

function buildPrPackagesComment({ repository, runId, commitSha, packages }) {
  const repo = String(repository || "").trim();
  const run = String(runId || "").trim();
  const sha = String(commitSha || "").trim();

  if (!repo) {
    throw new Error("repository is required");
  }
  if (!run || !/^\d+$/.test(run)) {
    throw new Error(`runId must be numeric, got "${runId}"`);
  }
  if (!sha) {
    throw new Error("commitSha is required");
  }

  const runUrl = `https://github.com/${repo}/actions/runs/${run}`;
  const lines = [
    COMMENT_MARKER,
    "## Test Packages",
    "",
    `- Commit: \`${sha}\``,
    `- Workflow run: [${run}](${runUrl})`,
    "",
    "Download:",
  ];

  for (const item of packages) {
    const label = item.variant ? `${item.variant}/${item.label}` : item.label;
    lines.push(`- ${label}: [\`${item.packageFile}\`](${runUrl}/artifacts/${item.artifactId})`);
  }

  return `${lines.join("\n")}\n`;
}

function parseArgs(argv) {
  const out = {};
  for (let idx = 2; idx < argv.length; idx += 2) {
    const key = argv[idx];
    const value = argv[idx + 1];
    if (!key || !key.startsWith("--")) {
      throw new Error(`Unexpected argument "${key}"`);
    }
    if (value === undefined || value.startsWith("--")) {
      throw new Error(`Missing value for argument "${key}"`);
    }
    out[key.slice(2)] = value;
  }
  return out;
}

function runCli() {
  const args = parseArgs(process.argv);
  const metadataDir = args["metadata-dir"] || "pr-meta";
  const repository = args.repository || process.env.GITHUB_REPOSITORY;
  const runId = args["run-id"] || process.env.GITHUB_RUN_ID;
  const commitSha = args["commit-sha"] || process.env.GITHUB_SHA;
  const outputPath = args.output || "pr-comment.md";

  const packages = loadPackageMetadata(metadataDir);
  const comment = buildPrPackagesComment({
    repository,
    runId,
    commitSha,
    packages,
  });
  fs.writeFileSync(outputPath, comment);
}

if (require.main === module) {
  runCli();
}

module.exports = {
  COMMENT_MARKER,
  buildPrPackagesComment,
  listMetadataFiles,
  loadPackageMetadata,
  parseMetadataFile,
};
