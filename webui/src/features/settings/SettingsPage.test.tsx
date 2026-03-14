import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { SessionProvider } from '../../app/session';
import { SettingsPage } from './SettingsPage';

function renderSettingsPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });

  render(
    <QueryClientProvider client={queryClient}>
      <SessionProvider>
        <MemoryRouter>
          <SettingsPage />
        </MemoryRouter>
      </SessionProvider>
    </QueryClientProvider>,
  );
}

describe('SettingsPage', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('carrier_token', 'test-token');
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url);
      if (url.endsWith('/healthz')) {
        return new Response(JSON.stringify({ status: 'ok' }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/api/v1/features')) {
        return new Response(JSON.stringify({ features: {}, authz: {} }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/api/v1/telegram/transport')) {
        return new Response(JSON.stringify({ transport: { selected_mode: 'webhook', reason_code: 'READY' } }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/api/v1/auth/providers')) {
        return new Response(JSON.stringify({
          providers: [
            { id: 'openai', name: 'OpenAI', authMode: 'api_key', configured: true, reusable: false },
            { id: 'openai-codex', name: 'OpenAI Codex (OAuth)', authMode: 'oauth_device_code', configured: true, reusable: true, hasSavedCredential: true },
          ],
        }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/api/v1/channels')) {
        return new Response(JSON.stringify({
          channels: [
            { id: 'telegram', displayName: 'Telegram', configured: true, supportsPairing: true },
            { id: 'feishu', displayName: 'Feishu', configured: false, supportsPairing: false },
          ],
        }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      return new Response(JSON.stringify({}), { status: 200, headers: { 'Content-Type': 'application/json' } });
    }) as typeof fetch;
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    localStorage.clear();
  });

  test('renders unified provider and channel auth summaries from the new APIs', async () => {
    renderSettingsPage();

    await waitFor(() => expect(screen.getByText(/Configured channels:/i)).toBeInTheDocument());
    const summary = screen.getByText(/Configured channels:/i);
    expect(summary.textContent).toContain('Telegram');
    expect(summary.textContent).toContain('Reusable providers: OpenAI Codex (OAuth)');
    expect(summary.textContent).toContain('Configured providers: OpenAI, OpenAI Codex (OAuth)');
  });
});
