import { describe, expect, test } from 'vitest';
import { buildWorkerSummaryPayload, filterWorkers, normalizeWorkers } from './model';

const workers = [
  { id: 'w1', hostId: 'local', hostName: 'local', agentId: 'zeroclaw', source: 'local', state: 'busy', executionId: 'exec-1' },
  { id: 'w2', hostId: 'host-1', hostName: 'prod-a', agentId: 'picoclaw', source: 'remote', state: 'managed', stale: true },
  { id: 'w3', hostId: 'host-2', hostName: 'prod-b', agentId: 'zeroclaw', source: 'remote', state: 'error' },
];

describe('workers model', () => {
  test('normalizes workers payload', () => {
    expect(normalizeWorkers({ workers }).length).toBe(3);
    expect(normalizeWorkers({ workers: null })).toEqual([]);
  });

  test('builds worker summary', () => {
    expect(buildWorkerSummaryPayload(workers, { queuedTasks: 2 })).toEqual({
      total: 3,
      active: 1,
      busy: 1,
      local: 1,
      remote: 2,
      stale: 1,
      queueSummary: { queuedTasks: 2 },
    });
  });

  test('filters workers by search and state', () => {
    expect(filterWorkers(workers, 'prod-a', 'all').map((item) => item.id)).toEqual(['w2']);
    expect(filterWorkers(workers, '', 'stale').map((item) => item.id)).toEqual(['w2']);
    expect(filterWorkers(workers, '', 'active').map((item) => item.id)).toEqual(['w1']);
  });
});
