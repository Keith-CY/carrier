import type { ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import type { QuickLaunchDraft } from './model';
import { useQuickLaunchMutations } from './useQuickLaunchMutations';

const apiPost = vi.fn();
const navigate = vi.fn();

vi.mock('../../lib/api', () => ({
  apiPost: (...args: unknown[]) => apiPost(...args),
}));

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom');
  return {
    ...actual,
    useNavigate: () => navigate,
  };
});

function wrapper({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return (
    <QueryClientProvider client={queryClient}>
      {children}
    </QueryClientProvider>
  );
}

describe('useQuickLaunchMutations', () => {
  const quickLaunchDraft: QuickLaunchDraft = {
    selectedPresetId: 'incident-diagnosis',
    goal: '',
    templateInputs: {
      service: 'checkout',
      environment: 'prod',
      incidentSummary: 'latency regression after deploy',
    },
    provider: '',
    maxConcurrency: '',
    hostLabels: '',
    selectedHosts: [],
  };

  beforeEach(() => {
    apiPost.mockReset();
    navigate.mockReset();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  test('forwards preview-derived template metadata when creating an execution', async () => {
    apiPost.mockImplementation(async (path: string, payload: any) => {
      if (path === '/api/v1/orchestrator/executions') {
        expect(payload).toMatchObject({
          goal: 'Diagnose incident for service checkout in prod. Summary: latency regression after deploy.',
          templateId: 'incident-diagnosis',
          templateVersion: 'v1',
          requestedProvider: 'openrouter',
          approvalScope: 'infrastructure_only',
          requiredMemory: ['shared:incident-response', 'shared:service-catalog'],
          distillOutputs: ['shared:incident-lessons'],
          maxConcurrency: 3,
        });
        return { execution: { id: 'exec-quick-launch-1' } };
      }
      if (path === '/api/v1/orchestrator/executions/exec-quick-launch-1/authorize') {
        return { result: 'ok' };
      }
      throw new Error(`unexpected apiPost call: ${path}`);
    });

    const setQuickLaunchPlan = vi.fn();
    const setQuickLaunchMessage = vi.fn();
    const { result } = renderHook(() => useQuickLaunchMutations({
      quickLaunchDraft,
      quickLaunchPlan: {
        goal: 'Diagnose incident for service checkout in prod. Summary: latency regression after deploy.',
        templateId: 'incident-diagnosis',
        templateVersion: 'v1',
        provider: 'openrouter',
        approvalScope: 'infrastructure_only',
        requiredMemory: ['shared:incident-response', 'shared:service-catalog'],
        distillOutputs: ['shared:incident-lessons'],
        requiredWorkers: [{ hostId: 'local', agentId: 'zeroclaw', count: 1 }],
        taskUnits: [{ id: 'task-1', input: 'Collect incident context', hostId: 'local', agentId: 'zeroclaw' }],
        maxConcurrency: 3,
      },
      setQuickLaunchPlan,
      setQuickLaunchMessage,
    }), { wrapper });

    await act(async () => {
      await result.current.runMutation.mutateAsync();
    });

    await waitFor(() => {
      expect(apiPost).toHaveBeenCalledWith('/api/v1/orchestrator/executions', expect.objectContaining({
        templateId: 'incident-diagnosis',
        templateVersion: 'v1',
        requiredMemory: ['shared:incident-response', 'shared:service-catalog'],
        distillOutputs: ['shared:incident-lessons'],
      }));
    });
    expect(apiPost).toHaveBeenCalledWith('/api/v1/orchestrator/executions/exec-quick-launch-1/authorize', expect.objectContaining({
      approved: true,
      actor: 'webui',
      maxConcurrency: 3,
    }));
    expect(navigate).toHaveBeenCalledWith('/executions/exec-quick-launch-1');
  });
});
