import { toFiniteNumber } from '../../lib/format';

export function operationGroup(name: string): string {
  const value = String(name || '').trim().toLowerCase();
  if (!value) return 'other';
  if (value.startsWith('remote_chat_')) return 'remote';
  return value.split('_')[0] || 'other';
}

export function operationIsAnomalous(operation: Record<string, any>): boolean {
  const failures = toFiniteNumber(operation?.failure);
  const successRate = toFiniteNumber(operation?.successRate);
  const avgLatencyMs = toFiniteNumber(operation?.avgLatencyMs);
  return failures > 0 || (successRate > 0 && successRate < 1) || avgLatencyMs >= 1000;
}

export function statusLineMetrics(remoteMetrics: any, orchestratorMetrics: any, anomaliesOnly: boolean, group: string) {
  const parts = [];
  const timestamp = String(remoteMetrics?.timestamp || orchestratorMetrics?.timestamp || '').trim();
  if (timestamp) parts.push(`Updated at ${timestamp}`);
  if (group !== 'all') parts.push(`group=${group}`);
  if (anomaliesOnly) parts.push('anomalies only');
  const rollout = String(remoteMetrics?.rollout?.state || '').trim();
  if (rollout) parts.push(`rollout=${rollout}`);
  const executionTotal = toFiniteNumber(orchestratorMetrics?.executions?.total);
  if (executionTotal > 0) parts.push(`executions=${executionTotal}`);
  const staleWorkers = toFiniteNumber(orchestratorMetrics?.workers?.stale);
  if (staleWorkers > 0) parts.push(`stale_workers=${staleWorkers}`);
  return parts.join(' · ');
}

export function topAttributionLabel(items: any[]): string {
  if (!Array.isArray(items) || !items.length) return 'none';
  const sorted = items
    .slice()
    .sort((left, right) => {
      const costDelta = toFiniteNumber(right?.estimatedCostUsd) - toFiniteNumber(left?.estimatedCostUsd);
      if (costDelta !== 0) return costDelta;
      const executionsDelta = toFiniteNumber(right?.executions) - toFiniteNumber(left?.executions);
      if (executionsDelta !== 0) return executionsDelta;
      return String(left?.label || left?.key || '').localeCompare(String(right?.label || right?.key || ''));
    });
  return String(sorted[0]?.label || sorted[0]?.key || 'none').trim() || 'none';
}

export function buildObservabilityDrillDown(type: 'team' | 'project' | 'template' | 'trigger', value: string) {
  const search = new URLSearchParams();
  if (type === 'template') {
    search.set('template', value);
  } else if (type === 'trigger') {
    search.set('trigger', value.includes(':') ? value.split(':', 1)[0] : value);
    search.set('search', value);
  } else {
    search.set('search', value);
  }
  return `/executions?${search.toString()}`;
}
