import type { ReactNode } from 'react';
import { cleanup, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { PWAProvider } from '@/app/pwa';
import { SessionProvider } from '@/app/session';
import { useQuickEntryData } from './useQuickEntryData';

function wrapper({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
  return (
    <QueryClientProvider client={queryClient}>
      <PWAProvider>
        <SessionProvider>
          <MemoryRouter>
            {children}
          </MemoryRouter>
        </SessionProvider>
      </PWAProvider>
    </QueryClientProvider>
  );
}

describe('useQuickEntryData', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('carrier_token', 'test-token');
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : undefined;
      const url = String(typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url);
      const method = init?.method || request?.method || 'GET';

      if (url.endsWith('/healthz')) {
        return new Response(JSON.stringify({ status: 'ok' }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/api/v1/features')) {
        return new Response(JSON.stringify({
          features: {
            remoteControlPlaneEnabled: true,
          },
          authz: {
            permissions: {
              viewExecutions: true,
              launchExecutions: true,
              approveExecutions: true,
              managePolicies: true,
              manageProviders: true,
              manageHosts: true,
            },
          },
        }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/api/v1/instances')) {
        return new Response(JSON.stringify({ instances: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/api/v1/work/projects')) {
        return new Response(JSON.stringify({ projects: [{ id: 'proj_alpha', name: 'Alpha' }] }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/api/v1/work/items')) {
        return new Response(JSON.stringify({ items: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/api/v1/work/runs')) {
        return new Response(JSON.stringify({ runs: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/api/v1/telegram/transport')) {
        return new Response(JSON.stringify({ transport: { selected_mode: 'webhook', reason_code: 'READY' } }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/api/v1/remote/metrics')) {
        return new Response(JSON.stringify({ summary: { running: 1 } }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/api/v1/auth/providers')) {
        return new Response(JSON.stringify({ providers: [{ id: 'openrouter', name: 'OpenRouter', configured: true, reusable: true }] }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/api/v1/channels')) {
        return new Response(JSON.stringify({ channels: [{ id: 'webui', displayName: 'WebUI', configured: true, supportsWebUI: true }] }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/api/v1/orchestrator/executions') && method === 'GET') {
        return new Response(JSON.stringify({
          executions: [
            {
              id: 'exec-ask',
              goal: 'Approve production remediation',
              status: 'pending_authorization',
              project: 'checkout',
              updatedAt: '2026-03-18T11:00:00Z',
              policy: {
                decision: 'ask',
                reason: 'Production run requires approval.',
              },
            },
            {
              id: 'exec-running',
              goal: 'Investigate checkout latency',
              status: 'running',
              project: 'carrier',
              updatedAt: '2026-03-18T10:00:00Z',
              policy: {
                decision: 'allow',
                summary: 'Still running.',
              },
            },
          ],
        }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/api/v1/orchestrator/executions/exec-ask/authorize')) {
        return new Response(JSON.stringify({ result: 'ok' }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/api/v1/orchestrator/executions/exec-running/cancel')) {
        return new Response(JSON.stringify({ result: 'ok' }), { status: 202, headers: { 'Content-Type': 'application/json' } });
      }
      return new Response(JSON.stringify({}), { status: 200, headers: { 'Content-Type': 'application/json' } });
    }) as typeof fetch;
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    localStorage.clear();
  });

  test('loads approvals and active executions, then posts approve/cancel actions', async () => {
    const { result } = renderHook(() => useQuickEntryData(), { wrapper });

    await waitFor(() => expect(result.current.approvals).toHaveLength(1));
    expect(result.current.activeExecutions).toHaveLength(1);

    await result.current.approveExecution('exec-ask');
    await result.current.cancelExecution('exec-running');

    expect(globalThis.fetch).toHaveBeenCalledWith(
      '/api/v1/orchestrator/executions/exec-ask/authorize',
      expect.objectContaining({ method: 'POST' }),
    );
    expect(globalThis.fetch).toHaveBeenCalledWith(
      '/api/v1/orchestrator/executions/exec-running/cancel',
      expect.objectContaining({ method: 'POST' }),
    );
  });
});
