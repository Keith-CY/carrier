import { describe, expect, test } from 'vitest';
import { filterExecutions, normalizeExecutions, selectExecutionId } from './model';

const executions = [
  {
    id: 'exec-1',
    goal: 'Investigate prod incident',
    status: 'running',
    updatedAt: '2026-03-11T10:02:00Z',
    team: 'platform',
    templateId: 'incident-triage',
    triggerSource: 'webhook',
  },
  {
    id: 'exec-2',
    goal: 'Nightly smoke',
    status: 'completed',
    updatedAt: '2026-03-11T10:01:00Z',
    project: 'carrier',
    templateId: 'rollout-smoke',
    triggerSource: 'schedule',
  },
  {
    id: 'exec-3',
    goal: 'Backfill audit export',
    status: 'failed',
    updatedAt: '2026-03-11T10:00:00Z',
    environment: 'prod',
    templateId: '',
    triggerSource: '',
  },
];

describe('executions model', () => {
  test('normalizes executions newest first', () => {
    expect(normalizeExecutions({ executions }).map((item) => item.id)).toEqual(['exec-1', 'exec-2', 'exec-3']);
  });

  test('filters executions by search, template, trigger, and status', () => {
    const filtered = filterExecutions(normalizeExecutions({ executions }), {
      searchValue: 'platform',
      statusFilter: 'active',
      templateFilter: 'incident-triage',
      triggerFilter: 'webhook',
    });

    expect(filtered.map((item) => item.id)).toEqual(['exec-1']);
  });

  test('preserves current selection when still visible and otherwise falls back', () => {
    const normalized = normalizeExecutions({ executions });
    expect(selectExecutionId({ routeExecutionId: 'exec-2', selectedExecutionId: 'exec-1', filteredExecutions: normalized })).toBe('exec-2');
    expect(selectExecutionId({ routeExecutionId: '', selectedExecutionId: 'exec-2', filteredExecutions: normalized })).toBe('exec-2');
    expect(selectExecutionId({ routeExecutionId: '', selectedExecutionId: 'missing', filteredExecutions: normalized })).toBe('exec-1');
  });
});
