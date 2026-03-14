export function formatDateTime(raw: unknown): string {
  const text = String(raw || '').trim();
  if (!text) return 'n/a';
  const parsed = new Date(text);
  if (Number.isNaN(parsed.getTime())) return text;
  return parsed.toLocaleString();
}

export function formatAgeSeconds(value: unknown): string {
  const seconds = Number(value || 0);
  if (!Number.isFinite(seconds) || seconds <= 0) return 'n/a';
  if (seconds < 60) return `${Math.round(seconds)}s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
  return `${(seconds / 3600).toFixed(seconds % 3600 === 0 ? 0 : 1)}h`;
}

export function toFiniteNumber(value: unknown, fallback = 0): number {
  const num = Number(value);
  return Number.isFinite(num) ? num : fallback;
}

export function formatPercent(value: unknown): string {
  return `${Math.round(toFiniteNumber(value, 0) * 100)}%`;
}

export function formatMilliseconds(value: unknown): string {
  return `${Math.round(toFiniteNumber(value, 0))}ms`;
}

export function formatUSD(value: unknown): string {
  return `$${toFiniteNumber(value, 0).toFixed(4)}`;
}

export function formatMetricsBreakdown(value: unknown): string {
  if (!value || typeof value !== 'object') return 'none';
  const entries = Object.entries(value as Record<string, unknown>)
    .map(([key, count]) => [String(key).trim(), toFiniteNumber(count, 0)] as const)
    .filter(([key, count]) => key && count > 0)
    .sort((left, right) => left[0].localeCompare(right[0]));
  if (!entries.length) return 'none';
  return entries.map(([key, count]) => `${key}=${count}`).join(', ');
}

export function parseCommaSeparatedValues(raw: string): string[] {
  return String(raw || '')
    .split(',')
    .map((value) => value.trim())
    .filter(Boolean);
}
