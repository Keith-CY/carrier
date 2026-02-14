const { classifyBucket } = require("./classify-bucket");

// Simple test runner
let passed = 0;
let failed = 0;

function assert(condition, message) {
  if (condition) {
    passed++;
  } else {
    failed++;
    console.error(`FAIL: ${message}`);
  }
}

function assertEqual(actual, expected, label) {
  assert(
    actual === expected,
    `${label}: expected "${expected}", got "${actual}"`,
  );
}

// --- Hotfix bucket ---
assertEqual(
  classifyBucket({ title: "fix login", labels: [{ name: "hotfix" }] }),
  "Hotfix",
  "hotfix label",
);
assertEqual(
  classifyBucket({ title: "fix login", labels: [{ name: "review-followup" }] }),
  "Hotfix",
  "review-followup label",
);
assertEqual(
  classifyBucket({ title: "fix login", labels: [{ name: "quickfix" }] }),
  "Hotfix",
  "quickfix label",
);
assertEqual(
  classifyBucket({ title: "[review-followup] PR #446: something" }),
  "Hotfix",
  "review-followup title pattern",
);
assertEqual(
  classifyBucket({ title: "test: add coverage for X" }),
  "Hotfix",
  "test: title prefix",
);
assertEqual(
  classifyBucket({ title: "[Phase 1][Risk] something", labels: [] }),
  "Hotfix",
  "[phase 1][risk] title prefix",
);

// --- Decomposition bucket ---
assertEqual(
  classifyBucket({ title: "some plan", labels: [{ name: "plan" }] }),
  "Decomposition",
  "plan label",
);
assertEqual(
  classifyBucket({ title: "work", labels: [{ name: "decomposition" }] }),
  "Decomposition",
  "decomposition label",
);
assertEqual(
  classifyBucket({ title: "[Plan] breakdown of feature X" }),
  "Decomposition",
  "[Plan] title prefix",
);
assertEqual(
  classifyBucket({ title: "[Task] implement Y" }),
  "Decomposition",
  "[Task] title prefix",
);
assertEqual(
  classifyBucket({ title: "[Phase 2] next milestone" }),
  "Decomposition",
  "[Phase N] title prefix",
);
assertEqual(
  classifyBucket({ title: "Release workflow follow-up tasks" }),
  "Decomposition",
  "release workflow follow-up title",
);
assertEqual(
  classifyBucket({ title: "[A1] sub-item" }),
  "Decomposition",
  "[A1] code pattern",
);

// --- Unscheduled bucket ---
assertEqual(
  classifyBucket({ title: "future work", labels: [{ name: "unscheduled" }] }),
  "Unscheduled",
  "unscheduled label",
);
assertEqual(
  classifyBucket({ title: "future work", labels: [{ name: "backlog" }] }),
  "Unscheduled",
  "backlog label",
);
assertEqual(
  classifyBucket({ title: "pending review of something" }),
  "Unscheduled",
  "pending in title",
);

// --- Fallback / edge cases ---
assertEqual(
  classifyBucket({ title: "random issue with no labels" }),
  "Hotfix",
  "fallback to Hotfix (no matching labels or patterns)",
);
assertEqual(
  classifyBucket({ title: "", labels: [] }),
  "Hotfix",
  "empty title and labels fallback",
);

console.log(`\nResults: ${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
