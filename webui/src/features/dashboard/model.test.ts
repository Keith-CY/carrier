import { describe, expect, test } from 'vitest';
import { buildQuickLaunchPreviewRequest, CUSTOM_GOAL_PRESET_ID, DEFAULT_QUICK_LAUNCH_TEMPLATE_ID, toggleHostSelection } from './model';

describe('dashboard model', () => {
  test('validates custom goal preset before preview', () => {
    const result = buildQuickLaunchPreviewRequest({
      selectedPresetId: CUSTOM_GOAL_PRESET_ID,
      goal: '   ',
      templateInputs: {},
      provider: '',
      maxConcurrency: '',
      hostLabels: '',
      selectedHosts: ['local'],
    });

    expect(result).toEqual({ error: 'Goal is required.' });
  });

  test('prefers host labels over explicit host ids and normalizes payload', () => {
    const result = buildQuickLaunchPreviewRequest({
      selectedPresetId: DEFAULT_QUICK_LAUNCH_TEMPLATE_ID,
      goal: '',
      templateInputs: { service: 'api' },
      provider: ' openrouter ',
      maxConcurrency: '4',
      hostLabels: ' prod, gpu , prod ',
      selectedHosts: ['local', 'host-1'],
    });

    expect(result).toEqual({
      payload: {
        goal: '',
        templateId: DEFAULT_QUICK_LAUNCH_TEMPLATE_ID,
        inputs: { service: 'api' },
        provider: 'openrouter',
        hostIds: [],
        hostLabels: ['gpu', 'prod'],
        maxConcurrency: 4,
      },
    });
  });

  test('toggles host selection deterministically', () => {
    expect(toggleHostSelection(['local', 'host-1'], 'host-1')).toEqual(['local']);
    expect(toggleHostSelection(['local'], 'host-1')).toEqual(['local', 'host-1']);
  });
});
