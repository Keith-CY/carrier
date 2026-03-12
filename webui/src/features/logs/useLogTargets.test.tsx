import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, test, vi } from 'vitest';
import { useLogTargets } from './useLogTargets';

const apiGet = vi.fn();

vi.mock('../../lib/api', () => ({
  apiGet: (...args: unknown[]) => apiGet(...args),
}));

describe('useLogTargets', () => {
  beforeEach(() => {
    apiGet.mockReset();
  });

  test('builds options from agents and instances and selects first option', async () => {
    apiGet.mockImplementation(async (path: string) => {
      if (path === '/api/v1/agents') {
        return [{ id: 'zeroclaw', runtimeState: 'running' }];
      }
      if (path === '/api/v1/instances') {
        return { instances: [{ id: 'local-main', agent_id: 'picoclaw', runtime_state: 'running' }] };
      }
      throw new Error(`unexpected path: ${path}`);
    });

    const { result } = renderHook(() => useLogTargets());

    await waitFor(() => {
      expect(result.current.options).toEqual([
        { value: 'zeroclaw', label: 'zeroclaw (running)' },
        { value: 'picoclaw', label: 'picoclaw [local-main, running]' },
      ]);
    });
    expect(result.current.selectedAgent).toBe('zeroclaw');
    expect(result.current.statusBase).toBe('Select an agent and click Connect.');
  });
});
