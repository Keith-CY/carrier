"use strict";

const STATE_MARKER_REGEX = /<!--\s*carrier-1h-state:([A-Za-z0-9+/=]+)\s*-->/g;
const STATE_MARKER_NAME = "carrier-1h-state";
const DELTA_COMMENT_MARKER = "<!-- carrier-hourly-audit-delta -->";

function toFiniteNumber(value, fallback = 0) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function normalizeState(input) {
  const state = input && typeof input === "object" ? input : {};
  const metrics = state.metrics && typeof state.metrics === "object" ? state.metrics : {};
  const docsSupersededHistory = Array.isArray(state.docsSupersededHistory) ? state.docsSupersededHistory : [];
  const prWatchdog = state.prWatchdog && typeof state.prWatchdog === "object" ? state.prWatchdog : {};

  return {
    version: 1,
    metrics: {
      docs_todo: toFiniteNumber(metrics.docs_todo, 0),
      core_todo: toFiniteNumber(metrics.core_todo, 0),
      docs_superseded: toFiniteNumber(metrics.docs_superseded, 0),
      test_files: toFiniteNumber(metrics.test_files, 0),
    },
    cleanRunStreak: toFiniteNumber(state.cleanRunStreak, 0),
    docsSupersededHistory: docsSupersededHistory
      .map((item) => ({
        ts: String(item?.ts || "").trim(),
        value: toFiniteNumber(item?.value, 0),
      }))
      .filter((item) => item.ts),
    prWatchdog,
  };
}

function stripStateMarker(body) {
  const source = String(body || "");
  return source.replace(STATE_MARKER_REGEX, "").trimEnd();
}

function extractStateFromBody(body) {
  const source = String(body || "");
  const markerMatch = source.match(/<!--\s*carrier-1h-state:([A-Za-z0-9+/=]+)\s*-->/);
  if (!markerMatch || !markerMatch[1]) {
    return normalizeState({});
  }

  try {
    const decoded = Buffer.from(markerMatch[1], "base64").toString("utf8");
    return normalizeState(JSON.parse(decoded));
  } catch (_error) {
    return normalizeState({});
  }
}

function encodeStateMarker(state) {
  const normalized = normalizeState(state);
  const payload = Buffer.from(JSON.stringify(normalized), "utf8").toString("base64");
  return `<!-- ${STATE_MARKER_NAME}:${payload} -->`;
}

function upsertStateMarker(body, state) {
  const cleanBody = stripStateMarker(body);
  const marker = encodeStateMarker(state);
  if (!cleanBody) {
    return `${marker}\n`;
  }
  return `${cleanBody}\n\n${marker}\n`;
}

function computeCleanRunGate(previousState, metrics, requiredRuns) {
  const prev = normalizeState(previousState);
  const nowMetrics = normalizeState({ metrics }).metrics;
  const targetRuns = Math.max(1, toFiniteNumber(requiredRuns, 3));
  const cleanRun = nowMetrics.docs_todo === 0 && nowMetrics.core_todo === 0;
  const cleanRunStreak = cleanRun ? prev.cleanRunStreak + 1 : 0;

  return {
    cleanRun,
    cleanRunStreak,
    requiredRuns: targetRuns,
    readyForReview: cleanRunStreak >= targetRuns,
  };
}

function computeMetricDrift(previousValue, currentValue, alertThreshold) {
  const current = toFiniteNumber(currentValue, 0);
  const prevParsed = Number(previousValue);
  const hasPrevious = Number.isFinite(prevParsed);
  const delta = hasPrevious ? current - prevParsed : 0;
  const threshold = Math.max(1, toFiniteNumber(alertThreshold, 5));
  return {
    hasPrevious,
    delta,
    alert: hasPrevious && Math.abs(delta) >= threshold,
  };
}

function computeDocsSupersededTrend(previousState, currentValue, nowIso, windowSize) {
  const prev = normalizeState(previousState);
  const current = toFiniteNumber(currentValue, 0);
  const maxPoints = Math.max(1, toFiniteNumber(windowSize, 24));

  const previousValue = prev.docsSupersededHistory.length
    ? prev.docsSupersededHistory[prev.docsSupersededHistory.length - 1].value
    : null;

  const nextHistory = prev.docsSupersededHistory
    .concat([{ ts: String(nowIso || new Date().toISOString()), value: current }])
    .slice(-maxPoints);

  return {
    previousValue,
    delta: previousValue == null ? 0 : current - previousValue,
    regression: previousValue != null && current > previousValue,
    history: nextHistory,
  };
}

function buildPrFingerprint(pr) {
  const number = toFiniteNumber(pr?.number, 0);
  const sha = String(pr?.headSha || "").trim();
  const changedFiles = toFiniteNumber(pr?.changedFiles, 0);
  const additions = toFiniteNumber(pr?.additions, 0);
  const deletions = toFiniteNumber(pr?.deletions, 0);
  return `${number}:${sha}:${changedFiles}:${additions}:${deletions}`;
}

function computePrWatchdog(openPrs, previousState, options = {}) {
  const normalizedState = normalizeState(previousState);
  const previousMap = normalizedState.prWatchdog && typeof normalizedState.prWatchdog === "object"
    ? normalizedState.prWatchdog
    : {};
  const thresholdRuns = Math.max(1, toFiniteNumber(options.staleThresholdRuns, 6));
  const now = options.now instanceof Date ? options.now : new Date();
  const nowIso = now.toISOString();
  const nowMs = now.getTime();

  const entries = [];
  const nextMap = {};

  for (const pr of openPrs || []) {
    const number = toFiniteNumber(pr?.number, 0);
    if (!number) {
      continue;
    }

    const key = String(number);
    const fingerprint = buildPrFingerprint(pr);
    const prevEntry = previousMap[key] && typeof previousMap[key] === "object" ? previousMap[key] : null;
    const unchangedRuns = prevEntry && prevEntry.fingerprint === fingerprint
      ? toFiniteNumber(prevEntry.unchangedRuns, 0) + 1
      : 0;

    const createdAt = new Date(pr?.createdAt || nowIso);
    const updatedAt = new Date(pr?.updatedAt || pr?.createdAt || nowIso);
    const openHours = Math.max(0, Math.floor((nowMs - createdAt.getTime()) / 3600000));
    const lastUpdatedHours = Math.max(0, Math.floor((nowMs - updatedAt.getTime()) / 3600000));
    const stale = unchangedRuns >= thresholdRuns;

    const entry = {
      number,
      title: String(pr?.title || "").trim(),
      url: String(pr?.url || "").trim(),
      headRef: String(pr?.headRef || "").trim(),
      headSha: String(pr?.headSha || "").trim(),
      changedFiles: toFiniteNumber(pr?.changedFiles, 0),
      additions: toFiniteNumber(pr?.additions, 0),
      deletions: toFiniteNumber(pr?.deletions, 0),
      openHours,
      lastUpdatedHours,
      unchangedRuns,
      stale,
    };

    entries.push(entry);
    nextMap[key] = {
      fingerprint,
      unchangedRuns,
      headSha: entry.headSha,
      changedFiles: entry.changedFiles,
      lastSeenAt: nowIso,
    };
  }

  entries.sort((a, b) => {
    if (a.stale !== b.stale) {
      return a.stale ? -1 : 1;
    }
    if (a.unchangedRuns !== b.unchangedRuns) {
      return b.unchangedRuns - a.unchangedRuns;
    }
    if (a.openHours !== b.openHours) {
      return b.openHours - a.openHours;
    }
    return a.number - b.number;
  });

  return {
    thresholdRuns,
    entries,
    staleEntries: entries.filter((entry) => entry.stale),
    nextMap,
  };
}

function isAutomationAuditPr(entry) {
  const headRef = String(entry?.headRef || "").toLowerCase();
  const title = String(entry?.title || "").toLowerCase();
  if (headRef.startsWith("auto/")) {
    return true;
  }
  return title.includes("audit snapshot") || title.includes("architecture audit");
}

function formatSignedDelta(delta, hasPrevious) {
  if (!hasPrevious) {
    return "n/a";
  }
  return `${delta >= 0 ? "+" : ""}${delta}`;
}

function buildAuditDeltaComment(params) {
  const {
    generatedAt,
    metrics,
    drifts,
    docsSupersededTrend,
    mergeGate,
    watchdogEntry,
  } = params || {};

  const safeMetrics = normalizeState({ metrics }).metrics;
  const safeDrifts = drifts || {};
  const safeTrend = docsSupersededTrend || {};
  const safeGate = mergeGate || {};

  return [
    DELTA_COMMENT_MARKER,
    `Hourly audit delta (${generatedAt || new Date().toISOString()})`,
    "",
    "- docs_todo: " + safeMetrics.docs_todo + " (delta " + formatSignedDelta(safeDrifts.docsTodo?.delta || 0, !!safeDrifts.docsTodo?.hasPrevious) + ")",
    "- core_todo: " + safeMetrics.core_todo + " (delta " + formatSignedDelta(safeDrifts.coreTodo?.delta || 0, !!safeDrifts.coreTodo?.hasPrevious) + ")",
    "- docs_superseded: " + safeMetrics.docs_superseded + " (delta " + formatSignedDelta(safeTrend.delta || 0, safeTrend.previousValue != null) + ")",
    "- test_files: " + safeMetrics.test_files + " (delta " + formatSignedDelta(safeDrifts.testFiles?.delta || 0, !!safeDrifts.testFiles?.hasPrevious) + ")",
    "",
    "Merge gate:",
    "- clean streak: " + toFiniteNumber(safeGate.cleanRunStreak, 0) + "/" + toFiniteNumber(safeGate.requiredRuns, 3),
    "- ready for final review: " + (safeGate.readyForReview ? "yes" : "no"),
    "",
    "Watchdog:",
    "- PR #" + toFiniteNumber(watchdogEntry?.number, 0) + " unchanged_runs=" + toFiniteNumber(watchdogEntry?.unchangedRuns, 0) +
      ", open_hours=" + toFiniteNumber(watchdogEntry?.openHours, 0) +
      ", last_updated_hours=" + toFiniteNumber(watchdogEntry?.lastUpdatedHours, 0),
    "",
    "Existing open audit PR detected; posting delta update instead of opening a duplicate PR.",
  ].join("\n");
}

module.exports = {
  DELTA_COMMENT_MARKER,
  buildAuditDeltaComment,
  buildPrFingerprint,
  computeCleanRunGate,
  computeDocsSupersededTrend,
  computeMetricDrift,
  computePrWatchdog,
  encodeStateMarker,
  extractStateFromBody,
  formatSignedDelta,
  isAutomationAuditPr,
  normalizeState,
  stripStateMarker,
  upsertStateMarker,
};
