import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { useRemoteChatTargetData } from './useRemoteChatTargetData';

const apiGet = vi.fn();

vi.mock('../../lib/api', () => ({
  apiGet: (...args: unknown[]) => apiGet(...args),
}));

describe('useRemoteChatTargetData', () => {
  beforeEach(() => {
    apiGet.mockReset();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  test('loads remote hosts and profiles by default, then switches to local instances', async () => {
    apiGet.mockImplementation(async (path: string) => {
      if (path === '/api/v1/remote/hosts') {
        return { hosts: [{ id: 'host-1', name: 'Tokyo' }] };
      }
      if (path === '/api/v1/provider-profiles') {
        return { profiles: [{ id: 'openrouter', name: 'OpenRouter' }] };
      }
      if (path === '/api/v1/remote/hosts/host-1/instances') {
        return { instances: [{ agentId: 'zeroclaw', runtimeState: 'running' }] };
      }
      if (path === '/api/v1/instances') {
        return { instances: [{ id: 'local-main', agent_id: 'picoclaw', runtime_state: 'running' }] };
      }
      throw new Error(`unexpected path: ${path}`);
    });

    const updateStatus = vi.fn();
    const { result } = renderHook(() => useRemoteChatTargetData(updateStatus));

    await waitFor(() => {
      expect(result.current.hosts).toEqual([{ value: 'host-1', label: 'Tokyo' }]);
      expect(result.current.instances).toEqual([{ value: 'zeroclaw', label: 'zeroclaw (running)' }]);
    });

    act(() => {
      result.current.onTargetChange('local');
    });

    await waitFor(() => {
      expect(result.current.instances).toEqual([
        { value: '', label: 'base-agent (fallback)' },
        { value: 'picoclaw', label: 'local-main (picoclaw, running)' },
      ]);
    });
    expect(updateStatus).toHaveBeenCalled();
  });
});
