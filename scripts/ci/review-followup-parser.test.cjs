"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");

const { dedupeNbsLines, extractNbsLines, shortTitle } = require("./review-followup-parser.cjs");

test("extractNbsLines captures only NBS-prefixed lines", () => {
  const body = [
    "Looks good overall.",
    "NBS: tighten parser validation",
    "random line",
    "  NBS: add tests for malformed inputs  ",
  ].join("\n");

  const extracted = extractNbsLines(body, "review:alice");
  assert.deepEqual(extracted, [
    { suggestion: "tighten parser validation", source: "review:alice" },
    { suggestion: "add tests for malformed inputs", source: "review:alice" },
  ]);
});

test("dedupeNbsLines deduplicates normalized suggestions and keeps stable order", () => {
  const deduped = dedupeNbsLines([
    { suggestion: "Tighten parser validation", source: "review:alice" },
    { suggestion: "tighten   parser   validation", source: "review:bob" },
    { suggestion: "Add action-pinning tests", source: "issue-comment:carol" },
    { suggestion: "add action-pinning tests", source: "review-comment:dave" },
  ]);

  assert.equal(deduped.length, 2);
  assert.equal(deduped[0].suggestion, "Tighten parser validation");
  assert.equal(deduped[1].suggestion, "Add action-pinning tests");
  assert.deepEqual(deduped[0].sources, ["review:alice", "review:bob"]);
  assert.deepEqual(deduped[1].sources, ["issue-comment:carol", "review-comment:dave"]);
});

test("shortTitle truncates long suggestions deterministically", () => {
  const long = "x".repeat(120);
  const shortened = shortTitle(long);
  assert.equal(shortened.length, 80);
  assert.ok(shortened.endsWith("..."));
});
