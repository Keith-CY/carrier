"use strict";

const fs = require("node:fs");
const path = require("node:path");
const childProcess = require("node:child_process");

const COMMENT_MARKER = "<!-- carrier-pr-coverage-report -->";

const COVERAGE_SOURCES = {
  daemon: {
    key: "daemon",
    headProfile: "daemon/coverage.out",
    baseProfile: "coverage-base-daemon.out",
  },
  root: {
    key: "root",
    headProfile: "coverage-root.out",
    baseProfile: "coverage-base-root.out",
  },
  baseagent: {
    key: "baseagent",
    headProfile: "coverage-baseagent.out",
    baseProfile: "coverage-base-baseagent.out",
  },
  gateway: {
    key: "gateway",
    headProfile: "coverage-gateway.out",
    baseProfile: "coverage-base-gateway.out",
  },
  shared: {
    key: "shared",
    headProfile: "coverage-shared.out",
    baseProfile: "coverage-base-shared.out",
  },
  profilesync: {
    key: "profilesync",
    headProfile: "coverage-profilesync.out",
    baseProfile: "coverage-base-profilesync.out",
  },
  codeagent: {
    key: "codeagent",
    headProfile: "coverage-codeagent.out",
    baseProfile: "coverage-base-codeagent.out",
  },
  webui: {
    key: "webui",
    headProfile: "coverage-webui.out",
    baseProfile: "coverage-base-webui.out",
  },
};

const MODULES = [
  {
    key: "daemon",
    label: "Daemon (Go)",
    sources: ["daemon"],
  },
  {
    key: "core",
    label: "Carrier Core (Go)",
    sources: ["root", "baseagent", "gateway", "shared", "profilesync", "codeagent"],
  },
  {
    key: "webui",
    label: "WebUI (Go)",
    sources: ["webui"],
  },
];

const ROOT_PREFIXES = ["cmd/", "configv2/", "internal/", "pkg/"];
const CORE_PREFIXES = ["baseagent/", "gateway/", "shared/", "profilesync/", "codeagent/"];

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

function createEmptyStats(exists = false) {
  return {
    exists,
    total: 0,
    covered: 0,
    byFile: new Map(),
  };
}

function parseCoverprofile(profilePath) {
  if (!fs.existsSync(profilePath)) {
    return createEmptyStats(false);
  }

  const stats = createEmptyStats(true);
  const lines = fs.readFileSync(profilePath, "utf8").split(/\r?\n/);
  for (const rawLine of lines) {
    const line = rawLine.trim();
    if (!line || line.startsWith("mode:")) {
      continue;
    }
    const parts = line.split(" ");
    if (parts.length < 3) {
      continue;
    }
    const count = Number(parts.pop());
    const stmts = Number(parts.pop());
    const location = parts.join(" ");
    const filePath = location.split(":", 1)[0];
    if (!Number.isFinite(stmts) || !Number.isFinite(count)) {
      continue;
    }
    const current = stats.byFile.get(filePath) || { total: 0, covered: 0 };
    current.total += stmts;
    if (count > 0) {
      current.covered += stmts;
    }
    stats.byFile.set(filePath, current);
    stats.total += stmts;
    if (count > 0) {
      stats.covered += stmts;
    }
  }

  return stats;
}

function mergeStats(parts, requireAll = true) {
  const merged = createEmptyStats(parts.length > 0);
  for (const part of parts) {
    merged.exists = requireAll ? merged.exists && part.exists : merged.exists || part.exists;
    merged.total += part.total;
    merged.covered += part.covered;
    for (const [filePath, fileStats] of part.byFile.entries()) {
      const current = merged.byFile.get(filePath) || { total: 0, covered: 0 };
      current.total += fileStats.total;
      current.covered += fileStats.covered;
      merged.byFile.set(filePath, current);
    }
  }
  return merged;
}

function pct(stats) {
  if (!stats.total) {
    return 0;
  }
  return (stats.covered * 100) / stats.total;
}

function formatDelta(headStats, baseStats) {
  if (!headStats.exists || !baseStats.exists) {
    return "n/a";
  }
  if (!baseStats.total && headStats.total) {
    return "new";
  }
  if (!headStats.total && !baseStats.total) {
    return "n/a";
  }
  return `${(pct(headStats) - pct(baseStats)) >= 0 ? "+" : ""}${(pct(headStats) - pct(baseStats)).toFixed(2)}%`;
}

function isRootModulePath(repoPath) {
  if (repoPath === "go.mod" || repoPath === "go.sum") {
    return true;
  }
  if (!repoPath.includes("/") && repoPath.endsWith(".go")) {
    return true;
  }
  return ROOT_PREFIXES.some((prefix) => repoPath.startsWith(prefix));
}

function coverageSourceForRepoPath(repoPath) {
  const cleanPath = String(repoPath || "").trim();
  if (!cleanPath) {
    return null;
  }
  if (cleanPath.startsWith("daemon/")) {
    return "daemon";
  }
  if (cleanPath.startsWith("webui/")) {
    return "webui";
  }
  for (const prefix of CORE_PREFIXES) {
    if (cleanPath.startsWith(prefix)) {
      return prefix.slice(0, -1);
    }
  }
  if (isRootModulePath(cleanPath)) {
    return "root";
  }
  return null;
}

function isCoverageRelevantPath(repoPath) {
  if (!coverageSourceForRepoPath(repoPath)) {
    return false;
  }
  return (
    repoPath.endsWith(".go") ||
    repoPath.endsWith("/go.mod") ||
    repoPath.endsWith("/go.sum") ||
    repoPath === "go.mod" ||
    repoPath === "go.sum"
  );
}

function loadChangedFiles(baseSha, headSha, repoRoot) {
  if (!String(baseSha || "").trim() || !String(headSha || "").trim()) {
    return [];
  }

  try {
    const output = childProcess.execFileSync(
      "git",
      ["diff", "--name-only", baseSha, headSha],
      {
        cwd: repoRoot,
        encoding: "utf8",
      },
    );
    return output
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter((line) => line && isCoverageRelevantPath(line));
  } catch (error) {
    console.error(`changedFiles: git diff failed for ${baseSha}..${headSha}: ${error.message}`);
    return [];
  }
}

function loadParsedProfiles(repoRoot) {
  const parsed = {};
  for (const source of Object.values(COVERAGE_SOURCES)) {
    parsed[source.key] = {
      head: parseCoverprofile(path.join(repoRoot, source.headProfile)),
      base: parseCoverprofile(path.join(repoRoot, source.baseProfile)),
    };
  }
  return parsed;
}

function toCoverprofilePath(repoPath) {
  return `carrier/${repoPath}`;
}

function pickScopedStats(module, parsedProfiles, changedFiles) {
  const relevantFiles = changedFiles.filter((repoPath) => module.sources.includes(coverageSourceForRepoPath(repoPath)));
  if (relevantFiles.length === 0) {
    return {
      scope: "full module",
      matchedFiles: [],
      head: mergeStats(module.sources.map((source) => parsedProfiles[source].head)),
      base: mergeStats(module.sources.map((source) => parsedProfiles[source].base)),
    };
  }

  const headParts = [];
  const baseParts = [];
  let headExists = true;
  let baseExists = true;

  for (const repoPath of relevantFiles) {
    const sourceKey = coverageSourceForRepoPath(repoPath);
    const coverPath = toCoverprofilePath(repoPath);
    const headSource = parsedProfiles[sourceKey].head;
    const baseSource = parsedProfiles[sourceKey].base;
    const headFileStats = headSource.byFile.get(coverPath);
    const baseFileStats = baseSource.byFile.get(coverPath);

    if (!headSource.exists || !headFileStats) {
      headExists = false;
    }
    if (!baseSource.exists) {
      baseExists = false;
    }

    headParts.push({
      exists: Boolean(headSource.exists && headFileStats),
      total: headFileStats ? headFileStats.total : 0,
      covered: headFileStats ? headFileStats.covered : 0,
      byFile: new Map(),
    });
    baseParts.push({
      exists: Boolean(baseSource.exists),
      total: baseFileStats ? baseFileStats.total : 0,
      covered: baseFileStats ? baseFileStats.covered : 0,
      byFile: new Map(),
    });
  }

  const head = mergeStats(headParts, false);
  const base = mergeStats(baseParts, false);
  head.exists = headExists;
  base.exists = baseExists;

  return {
    scope: "changed Go files",
    matchedFiles: relevantFiles,
    head,
    base,
  };
}

function buildCoverageSummary({ parsedProfiles, changedFiles }) {
  const visibleModules = changedFiles.length > 0
    ? MODULES.filter((module) => changedFiles.some((repoPath) => module.sources.includes(coverageSourceForRepoPath(repoPath))))
    : MODULES;

  const rows = [];
  const missingModules = [];
  let comparedModules = 0;
  let avgHeadSum = 0;
  let avgBaseSum = 0;

  const moduleRows = {};
  for (const module of visibleModules) {
    const scoped = pickScopedStats(module, parsedProfiles, changedFiles);
    moduleRows[module.key] = scoped;
    const hasHead = scoped.head.exists;
    const hasBase = scoped.base.exists;
    let status = "✅";
    if (!(hasHead && hasBase)) {
      status = "⚠️";
      missingModules.push(module.label);
    } else {
      comparedModules += 1;
      avgHeadSum += pct(scoped.head);
      avgBaseSum += pct(scoped.base);
    }
    rows.push(
      `| ${module.label} | ${scoped.scope} | ${pct(scoped.head).toFixed(2)}% | ${formatDelta(scoped.head, scoped.base)} | ${status} |`,
    );
  }

  const fullModuleStats = Object.fromEntries(
    MODULES.map((module) => [
      module.key,
      {
        head: mergeStats(module.sources.map((source) => parsedProfiles[source].head)),
        base: mergeStats(module.sources.map((source) => parsedProfiles[source].base)),
      },
    ]),
  );

  const productHead = mergeStats(MODULES.map((module) => fullModuleStats[module.key].head));
  const productBase = mergeStats(MODULES.map((module) => fullModuleStats[module.key].base));
  const productDelta = pct(productHead) - pct(productBase);
  const gateTriggered = productDelta <= -3.0;

  const avgHead = comparedModules ? avgHeadSum / comparedModules : 0;
  const avgBase = comparedModules ? avgBaseSum / comparedModules : 0;
  const avgDelta = avgHead - avgBase;

  const lines = [
    COMMENT_MARKER,
    "### 📊 Coverage Report",
    "",
    changedFiles.length > 0
      ? "**Scope:** Changed Go files in this PR. Coverage gate decisions still use full product coverage across Go modules."
      : "**Scope:** Full module coverage.",
    "",
    "| Module | Scope | Line Coverage | Δ vs base | Status |",
    "|---|:---|---:|:---:|:---|",
    ...rows,
    "",
    `**Average line coverage:** ${avgHead.toFixed(2)}%`,
    `**Base average line coverage:** ${avgBase.toFixed(2)}%`,
    `**Δ vs base:** ${avgDelta >= 0 ? "+" : ""}${avgDelta.toFixed(2)}%`,
    `**Compared modules:** Modules with head/base coverage: ${comparedModules}/${visibleModules.length}`,
    "",
    "### Action needed",
  ];

  if (gateTriggered) {
    lines.push(
      `- ❌ Full product coverage dropped by ${Math.abs(productDelta).toFixed(2)}% (threshold: 3.00%). Add tests before merge.`,
    );
  } else if (missingModules.length > 0) {
    lines.push(`- ⚠️ Coverage profiles missing for: ${missingModules.join(", ")}. Please rerun tests for complete stats.`);
  } else {
    lines.push("- ✅ No action needed.");
  }

  lines.push("");
  lines.push("- To recalc the report after fixes: open this workflow run and rerun **Coverage Comment** job.");
  lines.push("");
  lines.push("_Status legend: ✅ OK, ⚠️ module missing, ❌ module coverage run failed._");

  return {
    comment: `${lines.join("\n")}\n`,
    requestChanges: gateTriggered,
    productDelta,
    productCurrent: pct(productHead),
    visibleModules: visibleModules.map((module) => module.key),
    moduleRows,
  };
}

function runCli() {
  const args = parseArgs(process.argv);
  const repoRoot = path.resolve(args["repo-root"] || ".");
  const baseSha = args["base-sha"] || process.env.BASE_SHA;
  const headSha = args["head-sha"] || process.env.HEAD_SHA;
  const outputPath = path.resolve(repoRoot, args.output || "coverage-comment.md");
  const requestOutputPath = path.resolve(repoRoot, args["request-output"] || "coverage-request-changes.txt");
  const productDeltaOutputPath = path.resolve(repoRoot, args["product-delta-output"] || "coverage-product-delta.txt");
  const productCurrentOutputPath = path.resolve(repoRoot, args["product-current-output"] || "coverage-product-current.txt");

  const parsedProfiles = loadParsedProfiles(repoRoot);
  const changedFiles = loadChangedFiles(baseSha, headSha, repoRoot);
  const summary = buildCoverageSummary({ parsedProfiles, changedFiles });

  fs.writeFileSync(outputPath, summary.comment);
  fs.writeFileSync(requestOutputPath, summary.requestChanges ? "true" : "false");
  fs.writeFileSync(productDeltaOutputPath, `${summary.productDelta >= 0 ? "+" : ""}${summary.productDelta.toFixed(2)}`);
  fs.writeFileSync(productCurrentOutputPath, `${summary.productCurrent.toFixed(2)}`);
}

if (require.main === module) {
  runCli();
}

module.exports = {
  COMMENT_MARKER,
  COVERAGE_SOURCES,
  MODULES,
  buildCoverageSummary,
  coverageSourceForRepoPath,
  isCoverageRelevantPath,
  loadChangedFiles,
  loadParsedProfiles,
  mergeStats,
  parseArgs,
  parseCoverprofile,
  pct,
};
