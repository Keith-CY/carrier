import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';
import { CUSTOM_GOAL_PRESET_ID, DEFAULT_QUICK_LAUNCH_TEMPLATE_ID } from './model';
import { useQuickLaunchDraftState } from './useQuickLaunchDraftState';

describe('useQuickLaunchDraftState', () => {
  afterEach(() => {
    // hook state is instance-local; no global cleanup needed beyond unmount
  });

  test('updates quick launch draft fields and resets to defaults', () => {
    const { result } = renderHook(() => useQuickLaunchDraftState());

    act(() => {
      result.current.selectQuickLaunchPreset('rollout-smoke-check', {
        defaultLaunchConfig: {
          provider: 'openrouter',
          hostLabels: ['prod'],
          maxConcurrency: 2,
        },
      });
      result.current.setQuickLaunchGoal('Investigate checkout latency');
      result.current.setQuickLaunchProvider('anthropic');
      result.current.setQuickLaunchTemplateInput('service', 'checkout');
      result.current.setQuickLaunchHostLabels('prod,gpu');
      result.current.setQuickLaunchMaxConcurrency('2');
      result.current.toggleQuickLaunchHost('prod-host-1');
      result.current.setQuickLaunchAdvancedVisible(true);
      result.current.setQuickLaunchMessage({ type: 'info', text: 'Preview ready.' });
      result.current.setQuickLaunchPlan({ goal: 'Investigate checkout latency' });
    });

    expect(result.current.quickLaunchDraft.selectedPresetId).toBe('rollout-smoke-check');
    expect(result.current.quickLaunchDraft.goal).toBe('Investigate checkout latency');
    expect(result.current.quickLaunchDraft.provider).toBe('anthropic');
    expect(result.current.quickLaunchDraft.templateInputs.service).toBe('checkout');
    expect(result.current.quickLaunchDraft.hostLabels).toBe('prod,gpu');
    expect(result.current.quickLaunchDraft.maxConcurrency).toBe('2');
    expect(result.current.quickLaunchDraft.selectedHosts).toContain('prod-host-1');
    expect(result.current.quickLaunchAdvancedVisible).toBe(true);
    expect(result.current.quickLaunchMessage.text).toBe('Preview ready.');
    expect(result.current.quickLaunchPlan).toEqual({ goal: 'Investigate checkout latency' });

    act(() => {
      result.current.resetQuickLaunch();
    });

    expect(result.current.quickLaunchDraft.selectedPresetId).toBe(DEFAULT_QUICK_LAUNCH_TEMPLATE_ID);
    expect(result.current.quickLaunchDraft.goal).toBe('');
    expect(result.current.quickLaunchDraft.templateInputs).toEqual({});
    expect(result.current.quickLaunchDraft.provider).toBe('');
    expect(result.current.quickLaunchDraft.maxConcurrency).toBe('');
    expect(result.current.quickLaunchDraft.hostLabels).toBe('');
    expect(result.current.quickLaunchDraft.selectedHosts).toEqual([]);
    expect(result.current.quickLaunchPlan).toBeNull();
    expect(result.current.quickLaunchMessage.text).toBe('');
  });

  test('switching to custom goal preset clears inherited preset routing without losing the draft goal', () => {
    const { result } = renderHook(() => useQuickLaunchDraftState());

    act(() => {
      result.current.selectQuickLaunchPreset('incident-diagnosis', {
        defaultLaunchConfig: {
          provider: 'openrouter',
          hostLabels: ['prod'],
          maxConcurrency: 3,
        },
      });
      result.current.setQuickLaunchTemplateInput('service', 'checkout');
      result.current.setQuickLaunchGoal('Investigate checkout latency');
      result.current.toggleQuickLaunchHost('prod-host-1');
      result.current.selectQuickLaunchPreset(CUSTOM_GOAL_PRESET_ID);
    });

    expect(result.current.quickLaunchDraft.selectedPresetId).toBe(CUSTOM_GOAL_PRESET_ID);
    expect(result.current.quickLaunchDraft.goal).toBe('Investigate checkout latency');
    expect(result.current.quickLaunchDraft.templateInputs).toEqual({});
    expect(result.current.quickLaunchDraft.provider).toBe('');
    expect(result.current.quickLaunchDraft.maxConcurrency).toBe('');
    expect(result.current.quickLaunchDraft.hostLabels).toBe('');
    expect(result.current.quickLaunchDraft.selectedHosts).toEqual(['local']);
  });
});
