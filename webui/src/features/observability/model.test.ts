import { describe, expect, test } from 'vitest';
import { buildObservabilityDrillDown, operationGroup, operationIsAnomalous, statusLineMetrics, topAttributionLabel } from './model';

describe('observability model', () => {
  test('groups operations', () => {
    expect(operationGroup('remote_chat_stream')).toBe('remote');
    expect(operationGroup('host_check')).toBe('host');
    expect(operationGroup('')).toBe('other');
  });

  test('detects anomalous operations', () => {
    expect(operationIsAnomalous({ failure: 1 })).toBe(true);
    expect(operationIsAnomalous({ successRate: 0.5 })).toBe(true);
    expect(operationIsAnomalous({ avgLatencyMs: 1200 })).toBe(true);
    expect(operationIsAnomalous({ successRate: 1, avgLatencyMs: 100 })).toBe(false);
  });

  test('builds status line and drill-down hrefs', () => {
    expect(statusLineMetrics({ timestamp: 'now', rollout: { state: 'stable' } }, { executions: { total: 2 }, workers: { stale: 1 } }, true, 'host')).toContain('executions=2');
    expect(buildObservabilityDrillDown('template', 'rollout-smoke')).toBe('/executions?template=rollout-smoke');
    expect(buildObservabilityDrillDown('trigger', 'webhook:incident')).toBe('/executions?trigger=webhook&search=webhook%3Aincident');
  });

  test('picks top attribution label by cost then executions', () => {
    expect(topAttributionLabel([
      { label: 'a', estimatedCostUsd: 1, executions: 2 },
      { label: 'b', estimatedCostUsd: 2, executions: 1 },
    ])).toBe('b');
  });
});
