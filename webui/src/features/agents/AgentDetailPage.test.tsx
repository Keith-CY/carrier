import { cleanup, render, screen, waitFor } from '@testing-library/react';
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
  });
});
