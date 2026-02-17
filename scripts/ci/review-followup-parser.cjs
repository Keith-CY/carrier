"use strict";

const NBS_LINE_RE = /^\s*NBS:\s*(.+)\s*$/gim;

function normalizeSuggestion(text) {
  return String(text || "")
    .trim()
    .replace(/\s+/g, " ")
    .toLowerCase();
}

function extractNbsLines(text, source) {
  if (!text) {
    return [];
  }

  const out = [];
  let match;
  while ((match = NBS_LINE_RE.exec(text)) !== null) {
    const suggestion = String(match[1] || "").trim().replace(/\s+/g, " ");
    if (!suggestion) {
      continue;
    }
    out.push({
      suggestion,
      source,
    });
  }
  return out;
}

function dedupeNbsLines(extracted) {
  const bySuggestion = new Map();
  for (const item of extracted) {
    const key = normalizeSuggestion(item.suggestion);
    if (!key) {
      continue;
    }
    const existing = bySuggestion.get(key);
    if (existing) {
      existing.sources.add(item.source);
      continue;
    }
    bySuggestion.set(key, {
      suggestion: item.suggestion.trim().replace(/\s+/g, " "),
      sources: new Set([item.source]),
    });
  }

  return Array.from(bySuggestion.values()).map((item) => ({
    suggestion: item.suggestion,
    sources: Array.from(item.sources),
  }));
}

function shortTitle(text) {
  const cleaned = String(text || "").replace(/\s+/g, " ").trim();
  if (cleaned.length <= 80) {
    return cleaned;
  }
  return `${cleaned.slice(0, 77)}...`;
}

module.exports = {
  dedupeNbsLines,
  extractNbsLines,
  shortTitle,
};
