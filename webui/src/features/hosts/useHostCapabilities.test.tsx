import { renderHook } from '@testing-library/react';
import { describe, expect, test, vi } from 'vitest';
import { useHostCapabilities } from './useHostCapabilities';

const useFeatures = vi.fn();

vi.mock('../../app/useFeatures', () => ({
  useFeatures: (...args: unknown[]) => useFeatures(...args),
}));

describe('useHostCapabilities', () => {
  test('derives manage-host permissions from feature flags and authz', () => {
    useFeatures.mockReturnValue({
      featureFlags: { remoteControlPlaneEnabled: true },
      authz: { permissions: { manageHosts: true } },
      isLoading: false,
    });

    const { result } = renderHook(() => useHostCapabilities());

    expect(result.current.featuresLoading).toBe(false);
    expect(result.current.canManageHosts).toBe(true);
    expect(result.current.featureFlags.remoteControlPlaneEnabled).toBe(true);
  });
});
