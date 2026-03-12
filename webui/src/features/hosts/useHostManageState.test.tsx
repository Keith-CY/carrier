import { act, renderHook } from '@testing-library/react';
import { describe, expect, test } from 'vitest';
import { useHostManageState } from './useHostManageState';

describe('useHostManageState', () => {
  test('showManageHost resets runtime panes and selects host', () => {
    const hosts = [{ id: 'host-1', name: 'Tokyo' }];
    const { result } = renderHook(() => useHostManageState(hosts));

    act(() => {
      result.current.setInstancesText('instances');
      result.current.setLogsText('logs');
      result.current.setConfigText('config');
      result.current.setSessionsText('sessions');
      result.current.setMemoryText('memory');
      result.current.showManageHost('host-1');
    });

    expect(result.current.selectedHostId).toBe('host-1');
    expect(result.current.selectedHost?.id).toBe('host-1');
    expect(result.current.instancesText).toBe('');
    expect(result.current.logsText).toBe('');
    expect(result.current.configText).toBe('');
    expect(result.current.sessionsText).toBe('');
    expect(result.current.memoryText).toBe('');
  });
});
