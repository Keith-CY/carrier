"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");

const { formatViolations, scanWorkflowContent } = require("./check-action-pinning.cjs");

const PINNED_SHA = "0123456789abcdef0123456789abcdef01234567";

test("scanWorkflowContent allows commit-SHA refs and allowlisted/local actions", () => {
  const content = [
    "jobs:",
    "  ci:",
    "    steps:",
    `      - uses: vendor/action@${PINNED_SHA}`,
    "      - uses: actions/checkout@v4",
    "      - uses: github/codeql-action/init@v3",
    "      - uses: ./local/action",
  ].join("\n");

  const violations = scanWorkflowContent(content, ".github/workflows/example.yml");
  assert.equal(violations.length, 0);
});

test("scanWorkflowContent rejects mutable refs with actionable location", () => {
  const content = [
    "steps:",
    "  - uses: vendor/action@v1",
    "  - uses: vendor/action@v1.2.3",
    "  - uses: vendor/action@main",
  ].join("\n");

  const violations = scanWorkflowContent(content, ".github/workflows/mutable.yml");
  assert.equal(violations.length, 3);

  const rendered = formatViolations(violations);
  assert.match(rendered, /\.github\/workflows\/mutable\.yml:2/);
  assert.match(rendered, /\.github\/workflows\/mutable\.yml:3/);
  assert.match(rendered, /\.github\/workflows\/mutable\.yml:4/);
  assert.match(rendered, /vendor\/action@v1/);
  assert.match(rendered, /vendor\/action@v1\.2\.3/);
  assert.match(rendered, /vendor\/action@main/);
});

test("scanWorkflowContent rejects uses lines missing @ref", () => {
  const content = [
    "steps:",
    "  - uses: vendor/action",
  ].join("\n");

  const violations = scanWorkflowContent(content, ".github/workflows/missing-ref.yml");
  assert.equal(violations.length, 1);
  assert.match(violations[0].reason, /missing @ref/);
});
