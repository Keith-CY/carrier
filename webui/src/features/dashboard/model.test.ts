import { describe, expect, test } from 'vitest';
import { buildQuickLaunchPreviewRequest, toggleHostSelection } from './model';

describe('dashboard model', () => {
  test('validates goal mode before preview', () => {
    const result = buildQuickLaunchPreviewRequest({
      mode: 'goal',
      goal: '   ',
      templateId: '',
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
      mode: 'template',
      goal: '',
      templateId: 'incident-triage',
      templateInputs: { service: 'api' },
      provider: ' openrouter ',
      maxConcurrency: '4',
      hostLabels: ' prod, gpu , prod ',
      selectedHosts: ['local', 'host-1'],
    });

    expect(result).toEqual({
      payload: {
        goal: '',
        templateId: 'incident-triage',
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
