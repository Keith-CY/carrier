/**
 * classifyBucket — Classify a GitHub issue into a kanban bucket.
 *
 * Extracted from .github/workflows/carrier-one-hour-kanban.yml for testability.
 *
 * @param {{ title?: string; labels?: Array<{ name?: string }> }} item
 * @returns {'Hotfix' | 'Decomposition' | 'Unscheduled'}
 */
function classifyBucket(item) {
  const title = String(item.title || "").toLowerCase();
  const labels = new Set(
    (item.labels || []).map((label) => String(label.name || "").toLowerCase()),
  );

  if (
    labels.has("unscheduled") ||
    labels.has("backlog") ||
    labels.has("pending") ||
    title.includes("pending")
  ) {
    return "Unscheduled";
  }

  if (
    labels.has("review") ||
    labels.has("review-followup") ||
    labels.has("review followup") ||
    labels.has("review_followup") ||
    labels.has("quickfix") ||
    labels.has("hotfix") ||
    labels.has("test") ||
    title.includes("[review-followup]") ||
    title.includes("review-followup") ||
    title.startsWith("test:") ||
    title.startsWith("[phase 1][risk]")
  ) {
    return "Hotfix";
  }

  if (
    labels.has("plan") ||
    labels.has("planning") ||
    labels.has("decomposition") ||
    /^\[(plan|task)\]/i.test(item.title || "") ||
    /^\[task\]|^\[plan\]/i.test(item.title || "") ||
    /^\[phase [0-9]/i.test(item.title || "") ||
    /release\s+workflow\s+follow-up/i.test(title) ||
    /\[(?:A|B|C)\d+\]/i.test(item.title || "")
  ) {
    return "Decomposition";
  }

  return "Hotfix";
}

module.exports = { classifyBucket };
