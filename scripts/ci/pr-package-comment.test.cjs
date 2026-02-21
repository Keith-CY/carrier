"use strict";

const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");
const assert = require("node:assert/strict");

const {
  COMMENT_MARKER,
  buildPrPackagesComment,
  loadPackageMetadata,
} = require("./pr-package-comment.cjs");

function withTempDir(fn) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "carrier-pr-comment-"));
  try {
    return fn(dir);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

test("loadPackageMetadata reads sorted metadata and enforces zip package names", () => {
  withTempDir((dir) => {
    fs.writeFileSync(
      path.join(dir, "b.json"),
      JSON.stringify({
        label: "darwin-arm64",
        package_file: "carrier-darwin-arm64.zip",
        artifact_id: "22",
      }),
    );
    fs.writeFileSync(
      path.join(dir, "a.json"),
      JSON.stringify({
        label: "darwin-x64",
        package_file: "carrier-darwin-x64.zip",
        artifact_id: "11",
      }),
    );

    const metadata = loadPackageMetadata(dir);
    assert.equal(metadata.length, 2);
    assert.equal(metadata[0].artifactId, "11");
    assert.equal(metadata[1].artifactId, "22");
  });
});

test("loadPackageMetadata fails when package_file does not end with .zip", () => {
  withTempDir((dir) => {
    fs.writeFileSync(
      path.join(dir, "bad.json"),
      JSON.stringify({
        label: "linux-x64",
        package_file: "carrier-linux-x64",
        artifact_id: "9",
      }),
    );

    assert.throws(() => loadPackageMetadata(dir), /end with \.zip/);
  });
});

test("loadPackageMetadata discovers JSON files in nested directories", () => {
  withTempDir((dir) => {
    const sub = path.join(dir, "sub");
    fs.mkdirSync(sub);
    fs.writeFileSync(
      path.join(sub, "nested.json"),
      JSON.stringify({
        label: "linux-arm64",
        package_file: "carrier-linux-arm64.zip",
        artifact_id: "33",
      }),
    );
    fs.writeFileSync(
      path.join(dir, "top.json"),
      JSON.stringify({
        label: "linux-x64",
        package_file: "carrier-linux-x64.zip",
        artifact_id: "11",
      }),
    );

    const metadata = loadPackageMetadata(dir);
    assert.equal(metadata.length, 2);
  });
});

test("buildPrPackagesComment renders markdown body with artifact links", () => {
  const body = buildPrPackagesComment({
    repository: "Keith-CY/carrier",
    runId: "22252074017",
    commitSha: "38695c9610c2592d729cc566fa2b3619e0841905",
    packages: [
      { label: "darwin-arm64", packageFile: "carrier-darwin-arm64.zip", artifactId: "5600022400" },
      { label: "linux-x64", packageFile: "carrier-linux-x64.zip", artifactId: "5600022428" },
    ],
  });

  assert.match(body, new RegExp(COMMENT_MARKER.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  assert.match(body, /- Commit: `38695c9610c2592d729cc566fa2b3619e0841905`/);
  assert.match(body, /- Workflow run: \[22252074017\]\(https:\/\/github\.com\/Keith-CY\/carrier\/actions\/runs\/22252074017\)/);
  assert.match(body, /carrier-darwin-arm64\.zip/);
  assert.match(body, /actions\/runs\/22252074017\/artifacts\/5600022400/);
  assert.match(body, /carrier-linux-x64\.zip/);
  assert.match(body, /actions\/runs\/22252074017\/artifacts\/5600022428/);
});
