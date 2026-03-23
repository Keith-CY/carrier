import { render, screen } from '@testing-library/react';
import { describe, expect, test, vi } from 'vitest';
import { QuickLaunchCard } from './QuickLaunchCard';

describe('QuickLaunchCard', () => {
  test('renders preset-first quick launch without mode selector', () => {
    render(
      <QuickLaunchCard
        data={{
          featureFlags: { remoteControlPlaneEnabled: true },
          authz: { permissions: { launchExecutions: true } },
          quickLaunchMessage: { type: 'info', text: '' },
          quickLaunchDraft: {
            selectedPresetId: 'incident-diagnosis',
            goal: '',
            templateInputs: {},
            provider: '',
            maxConcurrency: '',
            hostLabels: '',
            selectedHosts: ['local'],
          },
          quickLaunchAdvancedVisible: false,
          quickLaunchPlan: null,
          previewMutation: { isPending: false },
          runMutation: { isPending: false },
          templates: [
            {
              id: 'incident-diagnosis',
              name: 'Incident Diagnosis',
              description: 'Diagnose incidents.',
              defaultLaunchConfig: { maxConcurrency: 3, approvalScope: 'infrastructure_only' },
              inputSchema: [{ id: 'service', label: 'Service', required: true }],
            },
          ],
          selectedTemplate: {
            id: 'incident-diagnosis',
            name: 'Incident Diagnosis',
            description: 'Diagnose incidents.',
            defaultLaunchConfig: { maxConcurrency: 3, approvalScope: 'infrastructure_only' },
            inputSchema: [{ id: 'service', label: 'Service', required: true }],
          },
          providerOptions: [],
          hostOptions: [{ id: 'local', name: 'local' }],
          selectQuickLaunchPreset: vi.fn(),
          setQuickLaunchGoal: vi.fn(),
          setQuickLaunchProvider: vi.fn(),
          setQuickLaunchMaxConcurrency: vi.fn(),
          setQuickLaunchHostLabels: vi.fn(),
          setQuickLaunchTemplateInput: vi.fn(),
          toggleQuickLaunchHost: vi.fn(),
          resetQuickLaunch: vi.fn(),
          setQuickLaunchAdvancedVisible: vi.fn(),
          clearQuickLaunchPreview: vi.fn(),
          previewQuickLaunch: vi.fn(),
          runQuickLaunch: vi.fn(),
        } as any}
      />,
    );

    expect(screen.queryByLabelText('Mode')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Incident Diagnosis' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Custom Goal' })).toBeInTheDocument();
    expect(screen.getByLabelText('Service')).toBeInTheDocument();
  });
});
