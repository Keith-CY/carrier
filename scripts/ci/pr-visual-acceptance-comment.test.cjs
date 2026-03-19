"use strict";

const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");
const assert = require("node:assert/strict");

const {
  COMMENT_MARKER,
  buildPrVisualAcceptanceComment,
  loadScreenshotCatalog,
} = require("./pr-visual-acceptance-comment.cjs");

function withTempDir(fn) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "carrier-visual-comment-"));
  try {
    return fn(dir);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

function touchPng(filePath) {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, "png");
}

test("loadScreenshotCatalog groups screenshots by flow and layout", () => {
  withTempDir((dir) => {
    touchPng(path.join(dir, "01-carrier-onboarding", "desktop", "00-welcome.png"));
    touchPng(path.join(dir, "01-carrier-onboarding", "mobile", "01-provider-step.png"));
    touchPng(path.join(dir, "04-multi-agent-orchestration", "pwa", "00-approval-handoff.png"));

    const catalog = loadScreenshotCatalog(dir);
    assert.equal(catalog.length, 5);
    assert.equal(catalog[0].layouts[0].shots[0].label, "Welcome");
    assert.equal(catalog[0].layouts[1].shots[0].label, "Provider Step");
    assert.equal(catalog[3].layouts[2].shots[0].relativePath, "04-multi-agent-orchestration/pwa/00-approval-handoff.png");
  });
});

test("buildPrVisualAcceptanceComment renders business lines with layout sections", () => {
  withTempDir((dir) => {
    touchPng(path.join(dir, "01-carrier-onboarding", "desktop", "00-welcome.png"));
    touchPng(path.join(dir, "01-carrier-onboarding", "mobile", "01-provider-step.png"));
    touchPng(path.join(dir, "01-carrier-onboarding", "pwa", "00-open-home.png"));

    const body = buildPrVisualAcceptanceComment({
      repository: "Keith-CY/carrier",
      runId: "22252074017",
      commitSha: "38695c9610c2592d729cc566fa2b3619e0841905",
      screenshotsRef: "screenshots/pr-1573",
      artifactUrl: "https://github.com/Keith-CY/carrier/actions/runs/22252074017/artifacts/5600022400",
      screenshotsDir: dir,
    });

    assert.match(body, new RegExp(COMMENT_MARKER.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
    assert.match(body, /### Carrier Onboarding/);
    assert.match(body, /<summary>Desktop<\/summary>/);
    assert.match(body, /<summary>Mobile<\/summary>/);
    assert.match(body, /<summary>PWA<\/summary>/);
    assert.match(body, /raw\.githubusercontent\.com\/Keith-CY\/carrier\/screenshots\/pr-1573\/01-carrier-onboarding\/desktop\/00-welcome\.png/);
    assert.match(body, /\| Welcome \|/);
    assert.match(body, /\| Provider Step \|/);
    assert.match(body, /Screenshot artifact: \[download\]/);
  });
});
