import { act, renderHook } from '@testing-library/react';
import { describe, expect, test } from 'vitest';
import { useRemoteChatStatus } from './useRemoteChatStatus';

describe('useRemoteChatStatus', () => {
  test('updates status text and type', () => {
    const { result } = renderHook(() => useRemoteChatStatus());

    expect(result.current.status).toBe('');
    expect(result.current.statusType).toBe('info');

    act(() => {
      result.current.updateStatus('Streaming response...', 'success');
    });

    expect(result.current.status).toBe('Streaming response...');
    expect(result.current.statusType).toBe('success');
  });
});
