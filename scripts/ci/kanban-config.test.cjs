"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");

const { parseKanbanConfig } = require("./kanban-config.cjs");

test("parseKanbanConfig accepts config with legacy projectId", () => {
  const config = parseKanbanConfig(JSON.stringify({
    projectId: "PVT_kwHOAG7zoc4BPLc6",
    fields: [],
  }), "with-project-id.json");

  assert.equal(config.projectId, "PVT_kwHOAG7zoc4BPLc6");
  assert.ok(Array.isArray(config.fields));
});

test("parseKanbanConfig accepts config without projectId", () => {
  const config = parseKanbanConfig(JSON.stringify({
    fields: [{ name: "Status" }],
  }), "without-project-id.json");

  assert.equal(config.projectId, undefined);
  assert.equal(config.fields.length, 1);
});

test("parseKanbanConfig rejects malformed JSON with clear source", () => {
  assert.throws(
    () => parseKanbanConfig("{not-json", "broken.json"),
    /Invalid JSON in broken\.json/,
  );
});

test("parseKanbanConfig rejects non-array fields", () => {
  assert.throws(
    () => parseKanbanConfig(JSON.stringify({ fields: "oops" }), "bad-fields.json"),
    /"fields" must be an array/,
  );
});
