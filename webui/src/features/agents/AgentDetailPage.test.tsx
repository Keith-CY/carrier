import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { SessionProvider } from '../../app/session';
import { AgentDetailPage } from './AgentDetailPage';

function renderAgentDetailPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });

  render(
    <QueryClientProvider client={queryClient}>
      <SessionProvider>
        <MemoryRouter initialEntries={['/agents/agent-alpha']}>
          <Routes>
            <Route path="/agents/:agentId" element={<AgentDetailPage />} />
          </Routes>
        </MemoryRouter>
      </SessionProvider>
    </QueryClientProvider>,
  );
}

describe('AgentDetailPage', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('carrier_token', 'test-token');
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url);
      if (url.endsWith('/api/v1/agents/agent-alpha/status')) {
        return new Response(JSON.stringify({ id: 'agent-alpha', runtimeState: 'running' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/capabilities')) {
        return new Response(JSON.stringify({
          skillSummary: {
            installedCount: 1,
            enabledCount: 1,
            disabledCount: 0,
          },
          skills: [
            { name: 'go-testing', enabled: true, summary: 'Use go test before claiming success.' },
          ],
          mcp: {
            servers: [
              { name: 'repo', health: 'healthy', visibleToolCount: 1, hiddenToolCount: 0 },
            ],
            visibleTools: [
              { name: 'repo_search', description: 'Search the repository index.' },
            ],
          },
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/launcher')) {
        return new Response(JSON.stringify({
          agentId: 'agent-alpha',
          heartbeat: { state: 'fresh', ageSeconds: 12 },
          memory: { contractId: 'memory-alpha' },
          providerReadiness: { provider: 'openrouter', ready: true, authMode: 'api_key' },
          modelSurface: {
            defaultProfile: 'openrouter-fast',
            profiles: [
              { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: true },
              { profileName: 'openrouter-safe', modelAlias: 'flash', modelId: 'deepseek/deepseek-chat-v3-0324', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: false },
            ],
          },
          cron: {
            count: 2,
            nextRunAt: '2026-03-13T00:00:00Z',
            lastRunAt: '2026-03-12T23:55:00Z',
            lastResult: 'succeeded',
            jobs: [
              { id: 'cron-1', prompt: 'check launcher', nextRunAt: '2026-03-13T00:00:00Z', lastResult: 'succeeded' },
            ],
          },
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/skills/go-testing')) {
        return new Response(JSON.stringify({
          skillSummary: {
            installedCount: 1,
            enabledCount: 0,
            disabledCount: 1,
          },
          skills: [
            { name: 'go-testing', enabled: false, summary: 'Use go test before claiming success.' },
          ],
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      return new Response(JSON.stringify({}), { status: 200, headers: { 'Content-Type': 'application/json' } });
    }) as typeof fetch;
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    localStorage.clear();
  });

  test('renders runtime capability sections for skills and MCP', async () => {
    renderAgentDetailPage();

    await waitFor(() => expect(screen.getByText('Runtime Capabilities')).toBeInTheDocument());
    expect(screen.getByText(/go-testing/i)).toBeInTheDocument();
    expect(screen.getByText(/repo_search/i)).toBeInTheDocument();
    expect(screen.getByText(/healthy/i)).toBeInTheDocument();
    expect(screen.getByText(/2 job\(s\)/i)).toBeInTheDocument();
    expect(screen.getByText(/check launcher/i)).toBeInTheDocument();
    expect(screen.getByText(/openrouter-fast/i)).toBeInTheDocument();
    expect(screen.getByText(/google\/gemini-2.0-flash-001/i)).toBeInTheDocument();
    expect(screen.getAllByText(/flash/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/openrouter:flash/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/group=2/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/1 installed · 1 enabled · 0 disabled/i)).toBeInTheDocument();
  });

  test('toggles skill state through the agent skill endpoint', async () => {
    renderAgentDetailPage();

    await waitFor(() => expect(screen.getByRole('button', { name: /disable/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /disable/i }));

    await waitFor(() => expect(screen.getByText(/Skill go-testing disabled\./i)).toBeInTheDocument());
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/skills/go-testing'),
      expect.objectContaining({
        method: 'POST',
      }),
    );
  });
});
