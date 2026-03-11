import { describe, expect, test } from 'vitest';
import { buildMemoryInstanceAction, buildMemorySearchPayload, normalizeMemoryPayload } from './model';

describe('memory model', () => {
  test('normalizes payload collections', () => {
    expect(normalizeMemoryPayload({ subject: 'agent-a', entries: [{}], attachments: 'x' }, 'fallback')).toEqual({
      subject: 'agent-a',
      entries: [{}],
      attachments: [],
      grants: [],
      audit: [],
    });
  });

  test('builds search payload and validates query', () => {
    expect(buildMemorySearchPayload({ subject: 'a', query: '   ', limit: '5', minScore: '0.4' })).toEqual({
      error: 'Search query is required.',
    });
    expect(buildMemorySearchPayload({ subject: 'a', query: 'fusion', limit: '5', minScore: '0.4' })).toEqual({
      payload: {
        subject: 'a',
        query: 'fusion',
        maxResults: 5,
        minScore: 0.4,
      },
    });
  });

  test('builds instance action payloads', () => {
    expect(buildMemoryInstanceAction({ action: 'attach', instanceId: '', scope: 'x', reason: '', dryRun: false })).toEqual({
      error: 'Instance ID is required.',
    });
    expect(buildMemoryInstanceAction({ action: 'attach', instanceId: 'inst-1', scope: '', reason: '', dryRun: false })).toEqual({
      error: 'Scope is required for attach/detach.',
    });
    expect(buildMemoryInstanceAction({ action: 'distill', instanceId: 'inst-1', scope: 'shared:profile', reason: 'promote', dryRun: true })).toEqual({
      path: '/api/v1/memory/instance/distill',
      payload: {
        instanceId: 'inst-1',
        scope: 'shared:profile',
        reason: 'promote',
        dryRun: true,
      },
    });
  });
});
