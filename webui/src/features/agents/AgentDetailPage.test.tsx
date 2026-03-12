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
    let cronPaused = false;
    let defaultProfile = 'openrouter-fast';
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
            { name: 'go-testing', enabled: true, summary: 'Use go test before claiming success.', source: 'catalog', version: 'builtin' },
          ],
          mcp: {
            servers: [
              { name: 'repo', health: 'healthy', enabled: true, manageable: true, visibleToolCount: 1, hiddenToolCount: 0 },
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
            defaultProfile,
            profiles: [
              { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: true },
              { profileName: 'openrouter-safe', modelAlias: 'flash-safe', modelId: 'deepseek/deepseek-chat-v3-0324', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: false },
            ],
          },
          cron: {
            count: 2,
            nextRunAt: '2026-03-13T00:00:00Z',
            lastRunAt: '2026-03-12T23:55:00Z',
            lastResult: 'succeeded',
            jobs: [
              { id: 'cron-1', prompt: 'check launcher', nextRunAt: '2026-03-13T00:00:00Z', lastResult: cronPaused ? 'paused' : 'succeeded', paused: cronPaused, history: [{ trigger: 'manual', result: 'succeeded' }] },
            ],
          },
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/models')) {
        return new Response(JSON.stringify({
          agentId: 'agent-alpha',
          instanceId: 'agent-alpha-main',
          configPath: '/tmp/agent-alpha/config.toml',
          modelSurface: {
            defaultProfile,
            profiles: defaultProfile === 'openrouter-safe'
              ? [
                  { profileName: 'openrouter-safe', modelAlias: 'flash-safe', modelId: 'deepseek/deepseek-chat-v3-0324', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: true },
                  { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: false },
                ]
              : [
                  { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: true },
                  { profileName: 'openrouter-safe', modelAlias: 'flash-safe', modelId: 'deepseek/deepseek-chat-v3-0324', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: false },
                ],
          },
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/models/sync')) {
        defaultProfile = 'openrouter-safe';
        return new Response(JSON.stringify({
          agentId: 'agent-alpha',
          instanceId: 'agent-alpha-main',
          configPath: '/tmp/agent-alpha/config.toml',
          synced: true,
          modelSurface: {
            defaultProfile: 'openrouter-safe',
            profiles: [
              { profileName: 'openrouter-safe', modelAlias: 'flash-safe', modelId: 'deepseek/deepseek-chat-v3-0324', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: true },
              { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: false },
            ],
          },
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/models/default')) {
        defaultProfile = 'openrouter-safe';
        return new Response(JSON.stringify({
          agentId: 'agent-alpha',
          instanceId: 'agent-alpha-main',
          configPath: '/tmp/agent-alpha/config.toml',
          modelSurface: {
            defaultProfile: 'openrouter-safe',
            profiles: [
              { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: true },
              { profileName: 'openrouter-safe', modelAlias: 'flash-safe', modelId: 'deepseek/deepseek-chat-v3-0324', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: false },
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
            { name: 'go-testing', enabled: false, summary: 'Use go test before claiming success.', source: 'catalog', version: 'builtin' },
          ],
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.includes('/api/v1/agents/agent-alpha/skills/search')) {
        return new Response(JSON.stringify({
          skills: [
            { name: 'workspace-inspection', summary: 'Inspect workspace state.', source: 'catalog', version: 'v1.2.3' },
          ],
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/skills/install')) {
        return new Response(JSON.stringify({
          name: 'workspace-inspection',
          summary: 'Inspect workspace state.',
          source: 'catalog',
          version: 'v1.2.3',
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/skills/uninstall')) {
        return new Response(JSON.stringify({
          name: 'go-testing',
          summary: 'Use go test before claiming success.',
          source: 'catalog',
          version: 'builtin',
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/mcp/repo')) {
        return new Response(JSON.stringify({
          mcp: {
            servers: [
              { name: 'repo', health: 'stopped', enabled: false, manageable: true, visibleToolCount: 1, hiddenToolCount: 0 },
            ],
            visibleTools: [],
          },
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/cron/cron-1/run')) {
        return new Response(JSON.stringify({ id: 'cron-1', prompt: 'check launcher', lastResult: 'succeeded', paused: false, history: [{ trigger: 'manual', result: 'succeeded' }] }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/cron/cron-1/pause')) {
        cronPaused = true;
        return new Response(JSON.stringify({ id: 'cron-1', prompt: 'check launcher', lastResult: 'paused', paused: true, history: [{ trigger: 'manual', result: 'succeeded' }] }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/cron/cron-1/resume')) {
        cronPaused = false;
        return new Response(JSON.stringify({ id: 'cron-1', prompt: 'check launcher', lastResult: 'resumed', paused: false, history: [{ trigger: 'manual', result: 'succeeded' }] }), {
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
    expect(screen.getAllByText(/openrouter-fast/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/google\/gemini-2.0-flash-001/i)).toBeInTheDocument();
    expect(screen.getAllByText(/flash/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/openrouter:flash/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/group=2/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/1 installed · 1 enabled · 0 disabled/i)).toBeInTheDocument();
    expect(screen.getAllByText(/catalog/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/builtin/i).length).toBeGreaterThan(0);
  });

  test('toggles skill state through the agent skill endpoint', async () => {
    renderAgentDetailPage();

    await waitFor(() => expect(screen.getByRole('button', { name: 'Disable' })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'Disable' }));

    await waitFor(() => expect(screen.getByText(/Skill go-testing disabled\./i)).toBeInTheDocument());
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/skills/go-testing'),
      expect.objectContaining({
        method: 'POST',
      }),
    );
  });

  test('searches and installs skills through managed skill endpoints', async () => {
    renderAgentDetailPage();

    await waitFor(() => expect(screen.getByPlaceholderText(/search skills/i)).toBeInTheDocument());
    fireEvent.change(screen.getByPlaceholderText(/search skills/i), { target: { value: 'workspace' } });
    fireEvent.click(screen.getByRole('button', { name: /search skills/i }));

    await waitFor(() => expect(screen.getByRole('button', { name: /install workspace-inspection/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /install workspace-inspection/i }));

    await waitFor(() => expect(screen.getByText(/Installed skill workspace-inspection\./i)).toBeInTheDocument());
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/skills/search?q=workspace'),
      expect.anything(),
    );
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/skills/install'),
      expect.objectContaining({
        method: 'POST',
      }),
    );
    expect(screen.getAllByText(/catalog/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/v1.2.3/i).length).toBeGreaterThan(0);
  });

  test('uninstalls skills through managed skill endpoint', async () => {
    renderAgentDetailPage();

    await waitFor(() => expect(screen.getByRole('button', { name: 'Uninstall' })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'Uninstall' }));

    await waitFor(() => expect(screen.getByText(/Removed skill go-testing\./i)).toBeInTheDocument());
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/skills/uninstall'),
      expect.objectContaining({
        method: 'POST',
      }),
    );
  });

  test('toggles MCP server state through the agent MCP endpoint', async () => {
    renderAgentDetailPage();

    await waitFor(() => expect(screen.getByRole('button', { name: /disable mcp/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /disable mcp/i }));

    await waitFor(() => expect(screen.getByText(/MCP server repo disabled\./i)).toBeInTheDocument());
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/mcp/repo'),
      expect.objectContaining({
        method: 'POST',
      }),
    );
  });

  test('runs, pauses, and resumes cron jobs through agent cron endpoints', async () => {
    renderAgentDetailPage();

    await waitFor(() => expect(screen.getByRole('button', { name: /Run cron-1 now/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /Run cron-1 now/i }));
    await waitFor(() => expect(screen.getByText(/Cron job cron-1 run requested\./i)).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: /Pause cron-1/i }));
    await waitFor(() => expect(screen.getByText(/Cron job cron-1 paused\./i)).toBeInTheDocument());
    await waitFor(() =>
      expect(screen.getByText(/One or more cron jobs are paused\. Resume or cancel them to restore scheduled automation\./i)).toBeInTheDocument(),
    );

    fireEvent.click(screen.getByRole('button', { name: /Resume cron-1/i }));
    await waitFor(() => expect(screen.getByText(/Cron job cron-1 resumed\./i)).toBeInTheDocument());

    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/cron/cron-1/run'),
      expect.objectContaining({ method: 'POST' }),
    );
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/cron/cron-1/pause'),
      expect.objectContaining({ method: 'POST' }),
    );
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/cron/cron-1/resume'),
      expect.objectContaining({ method: 'POST' }),
    );
  });

  test('renders remediation callouts for stale heartbeat and provider readiness', async () => {
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
          skillSummary: { installedCount: 0, enabledCount: 0, disabledCount: 0 },
          skills: [],
          mcp: { servers: [], visibleTools: [] },
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/launcher')) {
        return new Response(JSON.stringify({
          agentId: 'agent-alpha',
          heartbeat: { state: 'stale', ageSeconds: 240 },
          providerReadiness: { provider: 'openrouter', ready: false, authMode: 'api_key' },
          cron: {
            count: 1,
            jobs: [{ id: 'cron-1', prompt: 'check launcher', paused: true, lastResult: 'paused' }],
          },
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/models')) {
        return new Response(JSON.stringify({ agentId: 'agent-alpha', modelSurface: { profiles: [] } }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      return new Response(JSON.stringify({}), { status: 200, headers: { 'Content-Type': 'application/json' } });
    }) as typeof fetch;

    renderAgentDetailPage();

    await waitFor(() => expect(screen.getByText(/Remediation/i)).toBeInTheDocument());
    expect(screen.getByText(/Provider authentication is not ready\. Reconfigure credentials or switch to a ready profile\./i)).toBeInTheDocument();
    expect(screen.getByText(/Launcher heartbeat is stale\. Restart the agent or inspect the managed runtime\./i)).toBeInTheDocument();
    expect(screen.getByText(/One or more cron jobs are paused\. Resume or cancel them to restore scheduled automation\./i)).toBeInTheDocument();
  });

  test('loads dedicated model surface and syncs it on demand', async () => {
    renderAgentDetailPage();

    await waitFor(() => expect(screen.getByRole('button', { name: /sync models/i })).toBeInTheDocument());
    expect(screen.getByText(/\/tmp\/agent-alpha\/config.toml/i)).toBeInTheDocument();
    expect(screen.getAllByText(/openrouter-fast/i).length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole('button', { name: /sync models/i }));

    await waitFor(() => expect(screen.getByText(/Model surface synced\./i)).toBeInTheDocument());
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/models/sync'),
      expect.objectContaining({
        method: 'POST',
      }),
    );
  });

  test('switches default profile and refetches launcher/models', async () => {
    renderAgentDetailPage();

    await waitFor(() => expect(screen.getByRole('button', { name: /set default openrouter-safe/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /set default openrouter-safe/i }));

    await waitFor(() => expect(screen.getByText(/Default model profile set to openrouter-safe\./i)).toBeInTheDocument());
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/models/default'),
      expect.objectContaining({
        method: 'POST',
      }),
    );
    expect(screen.getByText(/default=openrouter-safe/i)).toBeInTheDocument();
  });
});
