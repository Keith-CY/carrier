import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
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
    let mcpAttached = true;
    let mcpConfig = '{"mode":"read"}';
    let updatedProfile = {
      profileName: 'openrouter-safe',
      modelAlias: 'flash-safe',
      modelId: 'deepseek/deepseek-chat-v3-0324',
      providerId: 'openrouter',
      protocolFamily: 'openai-compatible',
      baseUrl: '',
      authMethod: '',
      timeoutMs: 0,
      retryBudget: 0,
      fallbackStrategy: '',
      fallbackGroup: 'openrouter:flash',
      aliasGroupSize: 2,
      primary: false,
    };
    localStorage.clear();
    localStorage.setItem('carrier_token', 'test-token');
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
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
            {
              name: 'go-testing',
              enabled: true,
              summary: 'Use go test before claiming success.',
              source: 'catalog',
              provenance: 'managed update via catalog',
              version: 'builtin',
              targetVersion: 'v2.0.0',
              installedAt: '2026-03-12T00:00:00Z',
              updatedAt: '2026-03-12T00:04:00Z',
              health: 'degraded',
              healthDetail: 'Installed version builtin differs from target version v2.0.0.',
              remediationHint: 'Update skill to v2.0.0 or clear the target pin.',
              updateStatus: 'update_available',
              updateAvailable: true,
            },
          ],
          mcp: {
            servers: [
              { name: 'repo', health: mcpAttached ? 'healthy' : 'detached', enabled: true, attached: mcpAttached, manageable: true, visibleToolCount: 1, hiddenToolCount: 0 },
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
          mediaRuntime: { provider: 'openrouter', status: 'ready', detail: 'provider=openrouter runtime configured' },
        lastModelRun: {
          requestedAlias: 'flash-safe',
          requestedModel: 'deepseek/deepseek-chat-v3-0324',
          resolvedModel: 'deepseek/deepseek-chat-v3-0324',
          resolvedProfile: 'openrouter-safe',
          fallbackGroup: 'openrouter:flash',
          selectionStrategy: 'explicit_model',
          selectionOrdinal: 1,
          overrideHit: true,
          fallbackHit: true,
          lastRunAt: '2026-03-12T00:05:00Z',
          },
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
          delegation: {
            count: 1,
            jobs: [{ jobId: 'subagent-1', task: 'collect diagnostics', status: 'completed', result: 'done', updatedAt: '2026-03-12T00:06:00Z' }],
          },
          sessions: {
            count: 1,
            sessions: [{ key: 'telegram:alpha', messageCount: 8, summaryLength: 64, updatedAt: '2026-03-12T00:07:00Z' }],
          },
          remediations: [],
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/subagents/subagent-1')) {
        return new Response(JSON.stringify({
          jobId: 'subagent-1',
          task: 'collect diagnostics',
          status: 'completed',
          summary: 'summary-ready',
          result: 'done',
          updatedAt: '2026-03-12T00:06:00Z',
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
                  { ...updatedProfile, primary: true },
                  { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: false },
                ]
              : [
                  { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: true },
                  updatedProfile,
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
      if (url.endsWith('/api/v1/agents/agent-alpha/models/discover')) {
        return new Response(JSON.stringify({
          agentId: 'agent-alpha',
          instanceId: 'agent-alpha-main',
          configPath: '/tmp/agent-alpha/config.toml',
          driftState: 'drifted',
          driftReason: 'stored model surface differs from config-discovered model surface',
          modelSurface: {
            defaultProfile,
            profiles: [
              { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: true },
              updatedProfile,
            ],
          },
          discoveredModelSurface: {
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
      if (url.endsWith('/api/v1/agents/agent-alpha/models/profile')) {
        const body = JSON.parse(String(init?.body || '{}'));
        updatedProfile = {
          profileName: 'openrouter-safe',
          modelAlias: body.modelAlias,
          modelId: body.modelId,
          providerId: body.providerId,
          protocolFamily: body.providerId === 'anthropic' ? 'anthropic' : 'openai-compatible',
          baseUrl: body.baseUrl,
          authMethod: body.authMethod,
          timeoutMs: body.timeoutMs,
          retryBudget: body.retryBudget,
          fallbackStrategy: body.fallbackStrategy,
          fallbackGroup: 'anthropic:flash-safe-v2',
          aliasGroupSize: 1,
          primary: false,
        };
        return new Response(JSON.stringify({
          agentId: 'agent-alpha',
          instanceId: 'agent-alpha-main',
          configPath: '/tmp/agent-alpha/config.toml',
          modelSurface: {
            defaultProfile,
            profiles: [
              { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: true },
              updatedProfile,
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
            { name: 'go-testing', enabled: false, summary: 'Use go test before claiming success.', source: 'catalog', version: 'builtin', health: 'degraded', updateStatus: 'update_available', updateAvailable: true },
          ],
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.includes('/api/v1/agents/agent-alpha/skills/search')) {
        return new Response(JSON.stringify({
          skills: [
            { name: 'workspace-inspection', summary: 'Inspect workspace state.', source: 'catalog', version: 'v1.2.3', health: 'healthy', updateStatus: 'current', updateAvailable: false },
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
          health: 'healthy',
          updateStatus: 'current',
          updateAvailable: false,
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/skills/update')) {
        return new Response(JSON.stringify({
          name: 'go-testing',
          summary: 'Use go test before claiming success.',
          source: 'catalog',
          version: 'builtin',
          targetVersion: 'v2.0.0',
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
          targetVersion: 'v2.0.0',
          health: 'degraded',
          updateStatus: 'update_available',
          updateAvailable: true,
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/mcp/repo/attach')) {
        mcpAttached = true;
        return new Response(JSON.stringify({
          name: 'repo',
          health: 'healthy',
          enabled: true,
          attached: true,
          manageable: true,
          visibleToolCount: 1,
          hiddenToolCount: 1,
          healthDetail: 'connected to repository index',
          remediationHint: 'Disable MCP if repository indexing becomes noisy.',
          configDigest: 'sha256:cfg',
          configSummary: mcpConfig,
          visibleTools: [
            { name: 'repo_search', description: 'Search code' },
          ],
          hiddenTools: [
            { name: 'repo_admin', description: 'Admin index' },
          ],
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/mcp/repo/detach')) {
        mcpAttached = false;
        return new Response(JSON.stringify({
          name: 'repo',
          health: 'detached',
          enabled: true,
          attached: false,
          manageable: true,
          visibleToolCount: 1,
          hiddenToolCount: 1,
          healthDetail: 'detached from runtime',
          remediationHint: 'Attach MCP before expecting tools to appear.',
          configDigest: 'sha256:cfg',
          configSummary: mcpConfig,
          visibleTools: [
            { name: 'repo_search', description: 'Search code' },
          ],
          hiddenTools: [
            { name: 'repo_admin', description: 'Admin index' },
          ],
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/mcp/repo/config')) {
        const body = JSON.parse(String(init?.body || '{}'));
        mcpConfig = String(body.config || '');
        return new Response(JSON.stringify({
          name: 'repo',
          health: mcpAttached ? 'healthy' : 'detached',
          enabled: true,
          attached: mcpAttached,
          manageable: true,
          visibleToolCount: 1,
          hiddenToolCount: 1,
          healthDetail: 'connected to repository index',
          remediationHint: 'Disable MCP if repository indexing becomes noisy.',
          configDigest: 'sha256:test-config',
          configSummary: mcpConfig,
          visibleTools: [
            { name: 'repo_search', description: 'Search code' },
          ],
          hiddenTools: [
            { name: 'repo_admin', description: 'Admin index' },
          ],
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/api/v1/agents/agent-alpha/mcp/repo')) {
        if (String(init?.method || 'GET').toUpperCase() === 'POST') {
          return new Response(JSON.stringify({
            mcp: {
              servers: [
                { name: 'repo', health: 'stopped', enabled: false, attached: mcpAttached, manageable: true, visibleToolCount: 1, hiddenToolCount: 0 },
              ],
              visibleTools: [],
            },
          }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        return new Response(JSON.stringify({
          name: 'repo',
          health: mcpAttached ? 'healthy' : 'detached',
          enabled: true,
          attached: mcpAttached,
          manageable: true,
          visibleToolCount: 1,
          hiddenToolCount: 1,
          healthDetail: 'connected to repository index',
          remediationHint: 'Disable MCP if repository indexing becomes noisy.',
          configDigest: mcpConfig ? 'sha256:test-config' : '',
          configSummary: mcpConfig,
          visibleTools: [
            { name: 'repo_search', description: 'Search code' },
          ],
          hiddenTools: [
            { name: 'repo_admin', description: 'Admin index' },
          ],
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
    expect(screen.getAllByText(/go-testing/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/repo_search/i)).toBeInTheDocument();
    expect(screen.getByText(/healthy/i)).toBeInTheDocument();
    expect(screen.getByText(/2 job\(s\)/i)).toBeInTheDocument();
    expect(screen.getByText(/check launcher/i)).toBeInTheDocument();
    expect(screen.getAllByText(/openrouter-fast/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/google\/gemini-2.0-flash-001/i)).toBeInTheDocument();
    expect(screen.getAllByText(/flash/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/openrouter:flash/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/group=2/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/Model Runtime Trace/i)).toBeInTheDocument();
    expect(screen.getByText(/requested=flash-safe/i)).toBeInTheDocument();
    expect(screen.getByText(/resolved=deepseek\/deepseek-chat-v3-0324/i)).toBeInTheDocument();
    expect(screen.getByText(/override hit/i)).toBeInTheDocument();
    expect(screen.getByText(/fallback hit/i)).toBeInTheDocument();
    expect(screen.getByText(/last=2026-03-12T00:05:00Z/i)).toBeInTheDocument();
    expect(screen.getByText(/1 installed · 1 enabled · 0 disabled/i)).toBeInTheDocument();
    expect(screen.getAllByText(/catalog/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/managed update via catalog/i)).toBeInTheDocument();
    expect(screen.getAllByText(/builtin/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/target=v2.0.0/i)).toBeInTheDocument();
    expect(screen.getByText(/installed=2026-03-12T00:00:00Z/i)).toBeInTheDocument();
    expect(screen.getByText(/updated=2026-03-12T00:04:00Z/i)).toBeInTheDocument();
    expect(screen.getByText(/health=degraded/i)).toBeInTheDocument();
    expect(screen.getByText(/Installed version builtin differs from target version v2.0.0\./i)).toBeInTheDocument();
    expect(screen.getByText(/Update skill to v2.0.0 or clear the target pin\./i)).toBeInTheDocument();
    expect(screen.getByText(/update_available/i)).toBeInTheDocument();
    expect(screen.getByText(/Recent Delegation Jobs/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Inspect subagent-1 delegation/i })).toBeInTheDocument();
    expect(screen.getByText(/Recent Sessions/i)).toBeInTheDocument();
    expect(screen.getByText(/telegram:alpha/i)).toBeInTheDocument();
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

  test('updates skills with an explicit target version', async () => {
    renderAgentDetailPage();

    await waitFor(() => expect(screen.getByPlaceholderText(/Pin version for go-testing/i)).toBeInTheDocument());
    fireEvent.change(screen.getByPlaceholderText(/Pin version for go-testing/i), { target: { value: 'v2.0.0' } });
    fireEvent.click(screen.getByRole('button', { name: 'Update go-testing' }));

    await waitFor(() => expect(screen.getByText(/Updated skill go-testing\./i)).toBeInTheDocument());
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/skills/update'),
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ name: 'go-testing', version: 'v2.0.0' }),
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

  test('loads MCP server detail and remediation hints', async () => {
    renderAgentDetailPage();

    await waitFor(() => expect(screen.getByRole('button', { name: /Inspect repo MCP/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /Inspect repo MCP/i }));

    await waitFor(() => expect(screen.getByText(/connected to repository index/i)).toBeInTheDocument());
    expect(screen.getByText(/Disable MCP if repository indexing becomes noisy\./i)).toBeInTheDocument();
    expect(screen.getByText(/repo_admin/i)).toBeInTheDocument();
    expect(screen.getByText(/sha256:test-config/i)).toBeInTheDocument();
    expect(screen.getByDisplayValue(/\{"mode":"read"\}/i)).toBeInTheDocument();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/mcp/repo'),
      expect.anything(),
    );
  });

  test('loads delegated job detail from the managed endpoint', async () => {
    renderAgentDetailPage();

    await waitFor(() => expect(screen.getByRole('button', { name: /Inspect subagent-1 delegation/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /Inspect subagent-1 delegation/i }));

    await waitFor(() => expect(screen.getByText(/subagent-1 Delegation Detail/i)).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText(/summary-ready/i)).toBeInTheDocument());
    const detailCard = screen.getByText(/subagent-1 Delegation Detail/i).closest('.card');
    expect(detailCard).not.toBeNull();
    await waitFor(() => expect(within(detailCard as HTMLElement).getByText(/^done$/i)).toBeInTheDocument());
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/subagents/subagent-1'),
      expect.anything(),
    );
  });

  test('attaches and updates MCP server config through managed endpoints', async () => {
    renderAgentDetailPage();

    await waitFor(() => expect(screen.getByRole('button', { name: /Inspect repo MCP/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /Inspect repo MCP/i }));
    await waitFor(() => expect(screen.getByLabelText(/repo MCP config/i)).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: /Detach repo/i }));
    await waitFor(() => expect(screen.getByText(/MCP server repo detached\./i)).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: /Attach repo/i }));
    await waitFor(() => expect(screen.getByText(/MCP server repo attached\./i)).toBeInTheDocument());

    fireEvent.change(screen.getByLabelText(/repo MCP config/i), { target: { value: '{"mode":"write"}' } });
    fireEvent.click(screen.getByRole('button', { name: /Save repo config/i }));
    await waitFor(() => expect(screen.getByText(/MCP config for repo updated\./i)).toBeInTheDocument());

    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/mcp/repo/detach'),
      expect.objectContaining({ method: 'POST' }),
    );
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/mcp/repo/attach'),
      expect.objectContaining({ method: 'POST' }),
    );
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/mcp/repo/config'),
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ config: '{"mode":"write"}' }),
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
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
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
          status: { restartCount: 3, lastError: 'provider timeout', lastTriageSummary: 'restart via launcher' },
          heartbeat: { state: 'stale', ageSeconds: 240 },
          providerReadiness: { provider: 'openrouter', ready: false, authMode: 'api_key' },
          mediaRuntime: { provider: 'openrouter', status: 'unavailable', detail: 'provider=openrouter runtime unavailable', remediationHint: 'Configure transcription credentials or switch providers.' },
          cron: {
            count: 1,
            jobs: [{ id: 'cron-1', prompt: 'check launcher', paused: true, lastResult: 'paused' }],
          },
          remediations: [
            { category: 'provider', summary: 'Provider authentication is not ready. Reconfigure credentials or switch to a ready profile.', detail: 'provider=openrouter auth=api_key', action: { kind: 'sync-model-surface', label: 'Sync model surface' } },
            { category: 'heartbeat', summary: 'Launcher heartbeat is stale. Restart the agent or inspect the managed runtime.', detail: 'state=stale age=240s', action: { kind: 'start-runtime', label: 'Start runtime' } },
            { category: 'cron', summary: 'One or more cron jobs are paused. Resume or cancel them to restore scheduled automation.', detail: 'job=cron-1 last=paused', action: { kind: 'resume-cron', label: 'Resume cron-1', target: 'cron-1' } },
          ],
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
    expect(screen.getByText(/provider=openrouter auth=api_key/i)).toBeInTheDocument();
    expect(screen.getByText(/state=stale age=240s/i)).toBeInTheDocument();
    expect(screen.getByText(/job=cron-1 last=paused/i)).toBeInTheDocument();
    expect(screen.getByText(/restarts=3/i)).toBeInTheDocument();
    expect(screen.getByText(/lastError=provider timeout/i)).toBeInTheDocument();
    expect(screen.getByText(/triage=restart via launcher/i)).toBeInTheDocument();
    const remediationContainer = screen.getByText(/Remediation/i).parentElement;
    expect(remediationContainer).not.toBeNull();
    expect(within(remediationContainer as HTMLElement).getByRole('button', { name: /Sync model surface/i })).toBeInTheDocument();
    expect(within(remediationContainer as HTMLElement).getByRole('button', { name: /Start runtime/i })).toBeInTheDocument();
    expect(within(remediationContainer as HTMLElement).getByRole('button', { name: /Resume cron-1/i })).toBeInTheDocument();
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

  test('discovers managed model drift on demand', async () => {
    renderAgentDetailPage();

    await waitFor(() => expect(screen.getByRole('button', { name: /inspect model drift/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /inspect model drift/i }));

    await waitFor(() => expect(screen.getByText(/Model discovery drifted\./i)).toBeInTheDocument());
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/models/discover'),
      expect.objectContaining({
        method: 'GET',
      }),
    );
    expect(screen.getByText(/discovered-default=flash-safe -> deepseek\/deepseek-chat-v3-0324/i)).toBeInTheDocument();
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

  test('updates a managed profile and refetches launcher/models', async () => {
    renderAgentDetailPage();

    await waitFor(() => expect(screen.getByRole('button', { name: /edit profile openrouter-safe/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /edit profile openrouter-safe/i }));

    fireEvent.change(screen.getByLabelText(/model alias for openrouter-safe/i), { target: { value: 'flash-safe-v2' } });
    fireEvent.change(screen.getByLabelText(/model id for openrouter-safe/i), { target: { value: 'anthropic/claude-sonnet-4.6' } });
    fireEvent.change(screen.getByLabelText(/provider for openrouter-safe/i), { target: { value: 'anthropic' } });
    fireEvent.change(screen.getByLabelText(/base url for openrouter-safe/i), { target: { value: 'https://api.anthropic.com/v1' } });
    fireEvent.change(screen.getByLabelText(/auth method for openrouter-safe/i), { target: { value: 'api_key' } });
    fireEvent.change(screen.getByLabelText(/timeout ms for openrouter-safe/i), { target: { value: '60000' } });
    fireEvent.change(screen.getByLabelText(/retry budget for openrouter-safe/i), { target: { value: '4' } });
    fireEvent.change(screen.getByLabelText(/fallback strategy for openrouter-safe/i), { target: { value: 'round_robin' } });
    fireEvent.click(screen.getByRole('button', { name: /save profile openrouter-safe/i }));

    await waitFor(() => expect(screen.getByText(/Model profile openrouter-safe updated\./i)).toBeInTheDocument());
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/agents/agent-alpha/models/profile'),
      expect.objectContaining({
        method: 'POST',
        body: expect.stringContaining('"modelAlias":"flash-safe-v2"'),
      }),
    );
    expect(screen.getAllByText(/flash-safe-v2/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/anthropic\/claude-sonnet-4.6/i)).toBeInTheDocument();
  });
});
