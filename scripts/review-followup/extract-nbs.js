/**
 * Extract NBS (Non-Blocking Suggestion) lines from review text.
 *
 * Fixed: creates a new regex each call to avoid /g lastIndex state leaking
 * between invocations (see issue #36).
 */
function extractNbsLines(text) {
  if (!text) return [];
  const nbsRegex = /^\s*NBS:\s*(.+)\s*$/gmi;
  const out = [];
  let match;
  while ((match = nbsRegex.exec(text)) !== null) {
    const suggestion = (match[1] || "").trim();
    if (!suggestion) continue;
    out.push(suggestion);
  }
  return out;
}

module.exports = { extractNbsLines };
