"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const {
  collectCredentialLoaderDefinitionFiles,
  containsCredentialLoaderDefinition,
  validateCredentialLoaderDedup,
} = require("./credential-loader-dedupe.cjs");

test("containsCredentialLoaderDefinition detects known function names", () => {
  assert.equal(containsCredentialLoaderDefinition("func loadCredential() {}"), true);
  assert.equal(containsCredentialLoaderDefinition("func loadCredentialFromKeychain() {}"), true);
  assert.equal(containsCredentialLoaderDefinition("func loadCredentialFromFile() {}"), true);
  assert.equal(containsCredentialLoaderDefinition("func unrelated() {}"), false);
});

test("validateCredentialLoaderDedup reports unexpected files", () => {
  const result = validateCredentialLoaderDedup([
    "daemon/credentialstore/store.go",
    "daemon/internal/config/legacy.go",
  ]);

  assert.equal(result.hasAllowed, true);
  assert.deepEqual(result.unexpected, ["daemon/internal/config/legacy.go"]);
});

test("collectCredentialLoaderDefinitionFiles scans real files", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "carrier-loader-dedupe-"));
  try {
    const allowedDir = path.join(root, "daemon", "credentialstore");
    const otherDir = path.join(root, "daemon", "internal", "foo");
    fs.mkdirSync(allowedDir, { recursive: true });
    fs.mkdirSync(otherDir, { recursive: true });

    fs.writeFileSync(path.join(allowedDir, "store.go"), "package credentialstore\nfunc loadCredentialFromFile() {}\n");
    fs.writeFileSync(path.join(otherDir, "x.go"), "package foo\nfunc nope() {}\n");

    const files = collectCredentialLoaderDefinitionFiles(root);
    assert.equal(files.length, 1);
    assert.ok(files[0].endsWith("daemon/credentialstore/store.go"));
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});
