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

  assert.equal(lock.updated_at, "2026-03-19T00:00:00Z");
  assert.equal(openclaw.expected_fingerprint, "sha256:ab8b02dc9dad1c97bf97e8708900cdfabed8f463daea87dbc3d9197d9ef13c22");
  assert.deepEqual(trackedFilesByPath(openclaw), {
    "docs/gateway/configuration-reference.md": "49c743db6232ca0c61686b0e21a21eaa808ed436",
    "src/channels/plugins/config-schema.ts": "5ae166aa5a78066cdd3d1d21c8632ca6fcf88444",
    "src/config/config.ts": "3bd36d0d709594c7d896132c8472e445d1994940",
    "src/gateway/protocol/schema/config.ts": "9d0ec87666821aafd56049987c6bc7d54a5a3f72",
  });
});

test("upstreams lock records the refreshed picoclaw snapshot", () => {
  const lock = readLock();
  const picoclaw = lock.agents.picoclaw;

  assert.equal(picoclaw.expected_fingerprint, "sha256:00d9e819b162da93c6cc4f6f622cb12f525dc4d72c9b1b91d7cbe171be667203");
  assert.deepEqual(trackedFilesByPath(picoclaw), {
    "config/config.example.json": "221e89491a4e66013e42247ce8f1d559ce985bd0",
    "pkg/config/config.go": "4f8026d274d8b1f0b38a013c830e8d0e510496a6",
  });
});

test("upstreams lock records the refreshed zeroclaw snapshot and migrated doc path", () => {
  const lock = readLock();
  const zeroclaw = lock.agents.zeroclaw;

  assert.equal(zeroclaw.expected_fingerprint, "sha256:96aefe249019aa9c6f97efd0d762d15cb393252d505f27ed5e6c30b6915527ff");
  assert.deepEqual(trackedFilesByPath(zeroclaw), {
    "dev/config.template.toml": "cf4511b85d9c0f6ba8c1d5404fbfb168d30f7229",
    "docs/reference/api/config-reference.md": "5775c40d7266954a94b094b4cf2e5532bd415e8c",
    "src/config/schema.rs": "ad767b43ddf561e24f4b5c8b30db6fce06c904be",
  });
  assert.ok(!("docs/config-reference.md" in trackedFilesByPath(zeroclaw)));
});
