import { act, renderHook } from '@testing-library/react';
import { describe, expect, test } from 'vitest';
import { useProviderState } from './useProviderState';

describe('useProviderState', () => {
  test('hydrates default binding and preview host from lookups', () => {
    const { result } = renderHook(() =>
      useProviderState([{ id: 'openrouter' }, { id: 'anthropic' }], [{ id: 'host-a' }, { id: 'host-b' }]),
    );

    expect(result.current.bindingProfileId).toBe('openrouter');
    expect(result.current.profileTestHostId).toBe('host-a');
    expect(result.current.previewHostId).toBe('host-a');

    act(() => {
      result.current.setPreviewAgentId('picoclaw');
      result.current.setBindingTargetType('instance');
    });

    expect(result.current.previewAgentId).toBe('picoclaw');
    expect(result.current.bindingTargetType).toBe('instance');
  });
});
