"use strict";

const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");
const assert = require("node:assert/strict");

const {
  collectTauriBundleCandidates,
  getArchScore,
  selectTauriBundle,
} = require("./select-tauri-bundle.cjs");

function withTempDir(fn) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "carrier-tauri-bundle-"));
  try {
    return fn(dir);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

function touch(filePath) {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, "");
}

test("collectTauriBundleCandidates ignores cargo build executables outside release/bundle", () => {
  withTempDir((dir) => {
    touch(path.join(dir, "release", "build", "serde-json", "build-script-build.exe"));
    touch(path.join(dir, "release", "bundle", "nsis", "Carrier_0.1.0_x64-setup.exe"));

    const candidates = collectTauriBundleCandidates(dir);
    assert.equal(candidates.length, 1);
    assert.match(candidates[0].path, /release[\\/]+bundle[\\/]+nsis/);
    assert.match(candidates[0].path, /Carrier_0\.1\.0_x64-setup\.exe$/);
  });
});

test("selectTauriBundle prefers installer bundles for windows", () => {
  const selected = selectTauriBundle(
    [
      { path: "/tmp/target/release/bundle/nsis/Carrier_0.1.0_x64-setup.exe", ext: ".exe" },
      { path: "/tmp/target/release/bundle/msi/Carrier_0.1.0_x64_en-US.msi", ext: ".msi" },
      { path: "/tmp/target/release/bundle/updater/Carrier_0.1.0_x64.zip", ext: ".zip" },
    ],
    { goos: "windows", goarch: "amd64" },
  );

  assert.equal(selected.path, "/tmp/target/release/bundle/msi/Carrier_0.1.0_x64_en-US.msi");
});

test("selectTauriBundle prefers architecture-matching candidates when name encodes arch", () => {
  const selected = selectTauriBundle(
    [
      { path: "/tmp/target/release/bundle/rpm/Carrier-0.1.0-1.x86_64.rpm", ext: ".rpm" },
      { path: "/tmp/target/release/bundle/rpm/Carrier-0.1.0-1.aarch64.rpm", ext: ".rpm" },
    ],
    { goos: "linux", goarch: "arm64" },
  );

  assert.equal(selected.path, "/tmp/target/release/bundle/rpm/Carrier-0.1.0-1.aarch64.rpm");
});

test("collectTauriBundleCandidates includes .app bundles as directory outputs", () => {
  withTempDir((dir) => {
    touch(path.join(dir, "release", "bundle", "macos", "Carrier.app", "Contents", "Info.plist"));

    const candidates = collectTauriBundleCandidates(dir);
    assert.equal(candidates.length, 1);
    assert.match(candidates[0].path, /Carrier\.app$/);
    assert.equal(candidates[0].ext, ".app");
  });
});

test("getArchScore marks mismatched architecture tokens", () => {
  assert.equal(getArchScore("/tmp/Carrier_0.1.0_x64.dmg", "arm64"), 0);
  assert.equal(getArchScore("/tmp/Carrier_0.1.0_aarch64.dmg", "arm64"), 2);
  assert.equal(getArchScore("/tmp/Carrier.app", "arm64"), 1);
});
