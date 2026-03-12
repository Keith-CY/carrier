import { renderHook } from '@testing-library/react';
import { describe, expect, test } from 'vitest';
import { useSelectedHost } from './useSelectedHost';

describe('useSelectedHost', () => {
  test('returns the selected host when id matches', () => {
    const hosts = [
      { id: 'host-1', name: 'Tokyo' },
      { id: 'host-2', name: 'Osaka' },
    ];

    const { result, rerender } = renderHook(({ list, selectedHostId }) => useSelectedHost(list, selectedHostId), {
      initialProps: { list: hosts, selectedHostId: 'host-2' },
    });

    expect(result.current?.name).toBe('Osaka');

    rerender({ list: hosts, selectedHostId: 'missing' });
    expect(result.current).toBeNull();
  });
});
