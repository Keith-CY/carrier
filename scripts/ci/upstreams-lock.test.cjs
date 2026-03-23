"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

function readLock() {
  const lockPath = path.resolve(__dirname, "../../compat/upstreams.lock.json");
  return JSON.parse(fs.readFileSync(lockPath, "utf8"));
}

function trackedFilesByPath(agent) {
  return Object.fromEntries((agent.tracked_files || []).map((entry) => [entry.path, entry.sha]));
}

test("upstreams lock records the refreshed openclaw snapshot", () => {
  const lock = readLock();
  const openclaw = lock.agents.openclaw;

  assert.equal(lock.updated_at, "2026-03-21T00:00:00Z");
  assert.equal(openclaw.expected_fingerprint, "sha256:3ccbee7795afffb0c61ad943b87196371fa7f7ea7b967081ac305348469ba7c0");
  assert.deepEqual(trackedFilesByPath(openclaw), {
    "docs/gateway/configuration-reference.md": "11ea717513ad9d83fa82a3d7d5ced7bcf09e72a1",
    "src/channels/plugins/config-schema.ts": "5ae166aa5a78066cdd3d1d21c8632ca6fcf88444",
    "src/config/config.ts": "3bd36d0d709594c7d896132c8472e445d1994940",
    "src/gateway/protocol/schema/config.ts": "9d0ec87666821aafd56049987c6bc7d54a5a3f72",
  });
});

test("upstreams lock records the refreshed picoclaw snapshot", () => {
  const lock = readLock();
  const picoclaw = lock.agents.picoclaw;

  assert.equal(picoclaw.expected_fingerprint, "sha256:936b45c034be7dc575f3a95f5e2c02eb86f7b82f6e4a97c9290117cbf8db1b37");
  assert.deepEqual(trackedFilesByPath(picoclaw), {
    "config/config.example.json": "81c9014ec9e84d06c1ab307feb56d63a5e8216d3",
    "pkg/config/config.go": "235cb0641006d9c82a7f1fbf4e18d3106525836d",
  });
});

test("upstreams lock records the refreshed zeroclaw snapshot and migrated doc path", () => {
  const lock = readLock();
  const zeroclaw = lock.agents.zeroclaw;

  assert.equal(zeroclaw.expected_fingerprint, "sha256:52aed7b8ba46c3dba474a6716fb0148044d395313b4369f32184deb1b70256bc");
  assert.deepEqual(trackedFilesByPath(zeroclaw), {
    "dev/config.template.toml": "cf4511b85d9c0f6ba8c1d5404fbfb168d30f7229",
    "docs/reference/api/config-reference.md": "5436ff3db40d33b45cf3450e3737520dc4b0aefc",
    "src/config/schema.rs": "d8cb374b9e1245612303c83c8c3c1509d0a37a24",
  });
  assert.ok(!("docs/config-reference.md" in trackedFilesByPath(zeroclaw)));
});
