import { act, renderHook } from '@testing-library/react';
import { describe, expect, test } from 'vitest';
import { usePolicyState } from './usePolicyState';

describe('usePolicyState', () => {
  test('hydrates default template for trigger editor', () => {
    const { result } = renderHook(() => usePolicyState([{ id: 'incident-diagnosis' }, { id: 'pr-triage' }]));

    expect(result.current.triggerForm.templateId).toBe('incident-diagnosis');

    act(() => {
      result.current.setEditingTriggerId('trigger-1');
      result.current.setPolicyForm((current) => ({ ...current, name: 'ask-openrouter' }));
    });

    expect(result.current.editingTriggerId).toBe('trigger-1');
    expect(result.current.policyForm.name).toBe('ask-openrouter');
  });
});
