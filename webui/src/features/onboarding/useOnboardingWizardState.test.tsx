import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';
import { ensureWizardStateForRoute, resetWizardState } from './state';
import { useOnboardingWizardState } from './useOnboardingWizardState';

describe('useOnboardingWizardState', () => {
  afterEach(() => {
    resetWizardState();
  });

  test('initializes add-target state and persists wizard changes', () => {
    const { result } = renderHook(() => useOnboardingWizardState('setup', 'zeroclaw'));

    expect(result.current.baseState.addMode).toBe(true);
    expect(result.current.selectedAgent).toBe('zeroclaw');
    expect(result.current.channel).toBe('telegram');

    act(() => {
      result.current.setChannelToken('token-123');
      result.current.setWebhookSecret('secret-abc');
      result.current.setEnvRows([{ key: 'OPENAI_API_KEY', value: 'sk-test' }]);
    });

    const persisted = ensureWizardStateForRoute('setup', 'zeroclaw');
    expect(persisted.channelToken).toBe('token-123');
    expect(persisted.webhookSecret).toBe('secret-abc');
    expect(persisted.envRows).toEqual([{ key: 'OPENAI_API_KEY', value: 'sk-test' }]);
  });
});
