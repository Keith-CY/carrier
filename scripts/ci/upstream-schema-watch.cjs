#!/usr/bin/env node
"use strict";

const fs = require("fs");
const path = require("path");
const crypto = require("crypto");

function parseArgs(argv) {
  const out = {
    lockPath: "compat/upstreams.lock.json",
    mode: "check-only",
    failOnDrift: true,
  };
  for (let i = 2; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === "--lock" && argv[i + 1]) {
      out.lockPath = argv[i + 1];
      i += 1;
      continue;
    }
    if (arg === "--mode" && argv[i + 1]) {
      out.mode = argv[i + 1];
      i += 1;
      continue;
    }
    if (arg === "--no-fail-on-drift") {
      out.failOnDrift = false;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }
  if (!["check-only", "create-issues"].includes(out.mode)) {
    throw new Error(`unsupported mode: ${out.mode}`);
  }
  if (out.mode === "create-issues") {
    out.failOnDrift = false;
  }
  return out;
}

function loadLockFile(lockPath) {
  const abs = path.resolve(lockPath);
  const raw = fs.readFileSync(abs, "utf8");
  const parsed = JSON.parse(raw);
  if (!parsed || typeof parsed !== "object" || !parsed.agents || typeof parsed.agents !== "object") {
    throw new Error(`invalid lock file structure: ${abs}`);
  }
  return { absPath: abs, lock: parsed };
}

function repositoryOwnerAndName() {
  const fallback = "Keith-CY/carrier";
  const repo = String(process.env.GITHUB_REPOSITORY || fallback).trim() || fallback;
  const [owner, name] = repo.split("/");
  if (!owner || !name) {
    throw new Error(`invalid GITHUB_REPOSITORY: ${repo}`);
  }
  return { owner, repo: name, full: `${owner}/${name}` };
}

async function githubRequest(method, route, body) {
  const token = String(process.env.GITHUB_TOKEN || "").trim();
  const headers = {
    "Accept": "application/vnd.github+json",
    "User-Agent": "carrier-upstream-schema-watch",
  };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
  }

  const response = await fetch(`https://api.github.com${route}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  if (!response.ok) {
    const text = await response.text();
    throw new Error(`github api ${method} ${route} failed (${response.status}): ${text}`);
  }

  if (response.status === 204) {
    return null;
  }
  return await response.json();
}

function canonicalFingerprint(fileRows) {
  const lines = fileRows
    .map((row) => `${row.path}:${row.sha}`)
    .sort((a, b) => a.localeCompare(b));
  const payload = `${lines.join("\n")}\n`;
  const digest = crypto.createHash("sha256").update(payload, "utf8").digest("hex");
  return { digest: `sha256:${digest}`, lines };
}

async function loadRemoteFileSha(repository, filePath) {
  const encodedPath = filePath
    .split("/")
    .map((segment) => encodeURIComponent(segment))
    .join("/");
  const data = await githubRequest("GET", `/repos/${repository}/contents/${encodedPath}`);
  if (!data || typeof data.sha !== "string") {
    throw new Error(`missing sha for ${repository}:${filePath}`);
  }
  return data.sha.trim();
}

async function evaluateDrifts(lock) {
  const drifts = [];
  for (const [agentIDRaw, agent] of Object.entries(lock.agents || {})) {
    const agentID = String(agentIDRaw || "").trim().toLowerCase();
    if (!agentID) {
      continue;
    }
    const repository = String(agent.repository || "").trim();
    if (!repository) {
      throw new Error(`agent ${agentID}: repository is required`);
    }

    const trackedFiles = Array.isArray(agent.tracked_files) ? agent.tracked_files : [];
    if (!trackedFiles.length) {
      throw new Error(`agent ${agentID}: tracked_files is required`);
    }

    const remoteRows = [];
    for (const item of trackedFiles) {
      const filePath = String(item.path || "").trim();
      if (!filePath) {
        throw new Error(`agent ${agentID}: tracked_files.path is required`);
      }
      const remoteSha = await loadRemoteFileSha(repository, filePath);
      remoteRows.push({
        path: filePath,
        sha: remoteSha,
        expectedSha: String(item.sha || "").trim(),
      });
    }

    const actual = canonicalFingerprint(remoteRows);
    const expectedFingerprint = String(agent.expected_fingerprint || "").trim();
    const fingerprintChanged = expectedFingerprint !== "" && expectedFingerprint !== actual.digest;
    const fileChanges = remoteRows.filter((row) => row.expectedSha && row.expectedSha !== row.sha);

    if (fingerprintChanged || fileChanges.length > 0) {
      drifts.push({
        agentID,
        repository,
        expectedFingerprint,
        actualFingerprint: actual.digest,
        tracked: remoteRows,
        changedFiles: fileChanges,
        recommendedVersion: String(agent.recommended_version || "").trim(),
        renderers: Array.isArray(agent.supported_renderers) ? agent.supported_renderers : [],
      });
    }
  }
  return drifts;
}

function markerFor(agentID, fingerprint) {
  return `<!-- upstream-schema-watch:${agentID}:${fingerprint} -->`;
}

function buildIssueTitle(drift) {
  const short = drift.actualFingerprint.replace(/^sha256:/, "").slice(0, 12);
  return `[upstream-schema] ${drift.agentID}: schema drift detected (${short})`;
}

function buildIssueBody(drift) {
  const marker = markerFor(drift.agentID, drift.actualFingerprint);
  const rendererLines = drift.renderers.length
    ? drift.renderers.map((r) => `- ${r.id}: \`${r.version_range}\` -> \`${r.config_format}\``)
    : ["- (no renderer metadata in lock)"];

  const changedLines = drift.changedFiles.length
    ? drift.changedFiles.map((row) => `- \`${row.path}\`: \`${row.expectedSha || "(unset)"}\` -> \`${row.sha}\``)
    : drift.tracked.map((row) => `- \`${row.path}\`: \`${row.sha}\``);

  return [
    marker,
    `Upstream schema drift detected for **${drift.agentID}** from \`${drift.repository}\`.`,
    "",
    "## Snapshot",
    `- Expected fingerprint: \`${drift.expectedFingerprint || "(unset)"}\``,
    `- Current fingerprint: \`${drift.actualFingerprint}\``,
    drift.recommendedVersion ? `- Recommended version (lock): \`${drift.recommendedVersion}\`` : "- Recommended version (lock): (unset)",
    "",
    "## Changed Files",
    ...changedLines,
    "",
    "## Supported Renderers (Current Lock)",
    ...rendererLines,
    "",
    "## Follow-up Checklist",
    "- [ ] Verify upstream config/schema changes are intentional",
    "- [ ] Update Carrier renderer or version-range routing if needed",
    "- [ ] Update `compat/upstreams.lock.json` tracked SHAs/fingerprint",
    "- [ ] Add or update tests for new schema fields",
  ].join("\n");
}

async function listOpenIssues(owner, repo) {
  const issues = await githubRequest("GET", `/repos/${owner}/${repo}/issues?state=open&per_page=100`);
  return Array.isArray(issues) ? issues.filter((item) => !item.pull_request) : [];
}

async function ensureIssueForDrift(owner, repo, drift, openIssues) {
  const issueBodyMarkerPrefix = `<!-- upstream-schema-watch:${drift.agentID}:`;
  const exactMarker = markerFor(drift.agentID, drift.actualFingerprint);

  const existingExact = openIssues.find((issue) => String(issue.body || "").includes(exactMarker));
  if (existingExact) {
    console.log(`drift for ${drift.agentID} already tracked in issue #${existingExact.number}`);
    return;
  }

  const existingAgentIssue = openIssues.find((issue) => String(issue.body || "").includes(issueBodyMarkerPrefix));
  if (existingAgentIssue) {
    const comment = [
      `New drift fingerprint detected for \`${drift.agentID}\`.`,
      `- Previous lock fingerprint: \`${drift.expectedFingerprint || "(unset)"}\``,
      `- Current upstream fingerprint: \`${drift.actualFingerprint}\``,
      "",
      "Please refresh the issue body/checklist after adapting.",
      exactMarker,
    ].join("\n");

    await githubRequest("POST", `/repos/${owner}/${repo}/issues/${existingAgentIssue.number}/comments`, {
      body: comment,
    });
    console.log(`updated existing issue #${existingAgentIssue.number} for ${drift.agentID}`);
    return;
  }

  const title = buildIssueTitle(drift);
  const body = buildIssueBody(drift);
  const created = await githubRequest("POST", `/repos/${owner}/${repo}/issues`, { title, body });
  console.log(`created issue #${created.number} for ${drift.agentID}`);
}

async function run() {
  const args = parseArgs(process.argv);
  const { lock } = loadLockFile(args.lockPath);
  const drifts = await evaluateDrifts(lock);

  if (!drifts.length) {
    console.log("no upstream schema drift detected");
    return;
  }

  console.log(`detected ${drifts.length} upstream schema drift item(s)`);
  for (const drift of drifts) {
    console.log(`- ${drift.agentID}: ${drift.expectedFingerprint || "(unset)"} -> ${drift.actualFingerprint}`);
  }

  if (args.mode === "create-issues") {
    const { owner, repo } = repositoryOwnerAndName();
    const openIssues = await listOpenIssues(owner, repo);
    for (const drift of drifts) {
      await ensureIssueForDrift(owner, repo, drift, openIssues);
    }
    return;
  }

  if (args.failOnDrift) {
    process.exitCode = 1;
  }
}

run().catch((error) => {
  console.error(error.message || error);
  process.exitCode = 1;
});
