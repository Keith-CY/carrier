import { describe, expect, it } from 'vitest';
import {
  formatHostMeta,
  normalizeSSHAliases,
  parseCSV,
  pickRemoteInstanceAgentID,
} from './model';

describe('hosts model helpers', () => {
  it('dedupes and sorts csv labels', () => {
    expect(parseCSV('prod, gpu,prod , edge')).toEqual(['edge', 'gpu', 'prod']);
  });

  it('normalizes ssh aliases', () => {
    expect(normalizeSSHAliases({ hosts: ['beta', 'alpha', 'beta', ''] })).toEqual(['alpha', 'beta']);
  });

  it('picks remote instance agent id from explicit or composite ids', () => {
    expect(pickRemoteInstanceAgentID({ agentId: 'zeroclaw' })).toBe('zeroclaw');
    expect(pickRemoteInstanceAgentID({ id: 'host-1:picoclaw' })).toBe('picoclaw');
  });

  it('renders host metadata with operation info', () => {
    const output = formatHostMeta(
      { id: 'host-1', host: '10.0.0.1', user: 'ubuntu', labels: ['prod'], lastHealth: 'healthy' },
      { operation: 'host_check', success: true, durationMs: 1250, at: '2026-03-11T00:00:00Z' },
    );
    expect(output).toContain('endpoint: ubuntu@10.0.0.1');
    expect(output).toContain('labels: prod');
    expect(output).toContain('last op: host_check (ok)');
  });
});
