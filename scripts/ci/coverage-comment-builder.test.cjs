"use strict";

const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");
const assert = require("node:assert/strict");

const {
  buildCoverageSummary,
  coverageSourceForRepoPath,
  loadParsedProfiles,
} = require("./coverage-comment.cjs");

function withTempDir(fn) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "carrier-coverage-comment-"));
  try {
    return fn(dir);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

function writeCoverprofile(dir, relativePath, entries) {
  const fullPath = path.join(dir, relativePath);
  const lines = ["mode: set"];
  for (const entry of entries) {
    lines.push(`${entry.file}:1.1,1.2 ${entry.stmts} ${entry.count}`);
  }
  fs.mkdirSync(path.dirname(fullPath), { recursive: true });
  fs.writeFileSync(fullPath, `${lines.join("\n")}\n`);
}

test("coverageSourceForRepoPath maps nested modules and root paths correctly", () => {
  assert.equal(coverageSourceForRepoPath("daemon/internal/api/server.go"), "daemon");
  assert.equal(coverageSourceForRepoPath("baseagent/runtime.go"), "baseagent");
  assert.equal(coverageSourceForRepoPath("gateway/server.go"), "gateway");
  assert.equal(coverageSourceForRepoPath("shared/work/types.go"), "shared");
  assert.equal(coverageSourceForRepoPath("profilesync/git_repo.go"), "profilesync");
  assert.equal(coverageSourceForRepoPath("codeagent/runtime/runtime.go"), "codeagent");
  assert.equal(coverageSourceForRepoPath("cmd/carrier/main.go"), "root");
  assert.equal(coverageSourceForRepoPath("configv2/config.go"), "root");
  assert.equal(coverageSourceForRepoPath("go.mod"), "root");
  assert.equal(coverageSourceForRepoPath("webui/server.go"), "webui");
  assert.equal(coverageSourceForRepoPath("docs/coverage-report.md"), null);
});

test("buildCoverageSummary scopes Carrier Core to changed nested-module files", () => {
  withTempDir((dir) => {
    writeCoverprofile(dir, "coverage-root.out", [
      { file: "carrier/cmd/carrier/main.go", stmts: 100, count: 1 },
    ]);
    writeCoverprofile(dir, "coverage-base-root.out", [
      { file: "carrier/cmd/carrier/main.go", stmts: 100, count: 1 },
    ]);
    writeCoverprofile(dir, "coverage-baseagent.out", [
      { file: "carrier/baseagent/runtime.go", stmts: 10, count: 1 },
    ]);
    writeCoverprofile(dir, "coverage-base-baseagent.out", [
      { file: "carrier/baseagent/runtime.go", stmts: 10, count: 0 },
    ]);
    writeCoverprofile(dir, "daemon/coverage.out", [
      { file: "carrier/daemon/server/server.go", stmts: 20, count: 1 },
    ]);
    writeCoverprofile(dir, "coverage-base-daemon.out", [
      { file: "carrier/daemon/server/server.go", stmts: 20, count: 1 },
    ]);
    writeCoverprofile(dir, "coverage-webui.out", [
      { file: "carrier/webui/server.go", stmts: 5, count: 1 },
    ]);
    writeCoverprofile(dir, "coverage-base-webui.out", [
      { file: "carrier/webui/server.go", stmts: 5, count: 1 },
    ]);

    for (const name of [
      "coverage-gateway.out",
      "coverage-base-gateway.out",
      "coverage-shared.out",
      "coverage-base-shared.out",
      "coverage-profilesync.out",
      "coverage-base-profilesync.out",
      "coverage-codeagent.out",
      "coverage-base-codeagent.out",
    ]) {
      writeCoverprofile(dir, name, []);
    }

    const summary = buildCoverageSummary({
      parsedProfiles: loadParsedProfiles(dir),
      changedFiles: ["baseagent/runtime.go"],
    });

    assert.deepEqual(summary.visibleModules, ["core"]);
    assert.match(summary.comment, /\| Carrier Core \(Go\) \| changed Go files \| 100\.00% \| \+100\.00% \| ✅ \|/);
    assert.doesNotMatch(summary.comment, /Daemon \(Go\)/);
    assert.doesNotMatch(summary.comment, /cmd\/carrier\/main\.go/);
  });
});

test("buildCoverageSummary omits Carrier Core when only daemon files changed", () => {
  withTempDir((dir) => {
    writeCoverprofile(dir, "coverage-root.out", []);
    writeCoverprofile(dir, "coverage-base-root.out", []);
    writeCoverprofile(dir, "coverage-baseagent.out", []);
    writeCoverprofile(dir, "coverage-base-baseagent.out", []);
    writeCoverprofile(dir, "coverage-gateway.out", []);
    writeCoverprofile(dir, "coverage-base-gateway.out", []);
    writeCoverprofile(dir, "coverage-shared.out", []);
    writeCoverprofile(dir, "coverage-base-shared.out", []);
    writeCoverprofile(dir, "coverage-profilesync.out", []);
    writeCoverprofile(dir, "coverage-base-profilesync.out", []);
    writeCoverprofile(dir, "coverage-codeagent.out", []);
    writeCoverprofile(dir, "coverage-base-codeagent.out", []);
    writeCoverprofile(dir, "daemon/coverage.out", [
      { file: "carrier/daemon/internal/api/server.go", stmts: 10, count: 1 },
    ]);
    writeCoverprofile(dir, "coverage-base-daemon.out", [
      { file: "carrier/daemon/internal/api/server.go", stmts: 10, count: 1 },
    ]);
    writeCoverprofile(dir, "coverage-webui.out", []);
    writeCoverprofile(dir, "coverage-base-webui.out", []);

    const summary = buildCoverageSummary({
      parsedProfiles: loadParsedProfiles(dir),
      changedFiles: ["daemon/internal/api/server.go"],
    });

    assert.deepEqual(summary.visibleModules, ["daemon"]);
    assert.match(summary.comment, /\| Daemon \(Go\) \| changed Go files \| 100\.00% \| \+0\.00% \| ✅ \|/);
    assert.doesNotMatch(summary.comment, /Carrier Core \(Go\)/);
  });
});
