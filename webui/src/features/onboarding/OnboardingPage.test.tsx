import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { SessionProvider } from '../../app/session';
import { OnboardingPage } from './OnboardingPage';
import { resetWizardState } from './state';

function renderOnboardingSetup(addTargetAgent = 'picoclaw') {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });

  render(
    <QueryClientProvider client={queryClient}>
      <SessionProvider>
        <MemoryRouter>
          <OnboardingPage step="setup" addTargetAgent={addTargetAgent} />
        </MemoryRouter>
      </SessionProvider>
    </QueryClientProvider>,
  );
}

describe('OnboardingPage', () => {
  beforeEach(() => {
    resetWizardState();
    localStorage.clear();
    localStorage.setItem('carrier_token', 'test-token');
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url);
      if (url.endsWith('/healthz')) {
        return new Response(JSON.stringify({ status: 'ok' }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.includes('/api/v1/channels')) {
        return new Response(JSON.stringify({
          channels: [
            { id: 'telegram', displayName: 'Telegram', supportsPairing: true, requiresBotToken: true, requiresWebhookSecret: false, supportsProviderSetup: true },
            { id: 'feishu', displayName: 'Feishu', supportsPairing: false, requiresBotToken: true, requiresWebhookSecret: true, supportsProviderSetup: true },
          ],
        }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.includes('/api/v1/pairing/sessions?provider=telegram')) {
        return new Response(JSON.stringify({ sessions: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      return new Response(JSON.stringify({}), { status: 200, headers: { 'Content-Type': 'application/json' } });
    }) as typeof fetch;
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    resetWizardState();
    localStorage.clear();
  });

  test('loads channel choices from the channel status API and only requires pairing for pairing-capable channels', async () => {
    renderOnboardingSetup();

    await waitFor(() => expect(screen.getByRole('option', { name: 'Feishu' })).toBeInTheDocument());
    const channelSelect = screen.getByLabelText('Channel') as HTMLSelectElement;
    expect(channelSelect.value).toBe('telegram');

    const continueButton = screen.getByRole('button', { name: 'Continue →' }) as HTMLButtonElement;
    expect(continueButton.disabled).toBe(true);

    fireEvent.change(channelSelect, { target: { value: 'feishu' } });
    fireEvent.change(screen.getByLabelText('Channel Bot Token'), { target: { value: 'feishu-bot-token' } });
    fireEvent.change(screen.getByLabelText(/Webhook Secret/i), { target: { value: 'feishu-secret' } });

    await waitFor(() => expect(continueButton.disabled).toBe(false));
    expect(screen.queryByText(/Paired chat id:/i)).not.toBeInTheDocument();
  });

  test('does not require Telegram pairing when adding openclaw', async () => {
    renderOnboardingSetup('openclaw');

    await waitFor(() => expect(screen.getByRole('option', { name: 'Telegram' })).toBeInTheDocument());
    expect(screen.getByText(/No pairing required/i)).toBeInTheDocument();
    expect(screen.queryByText(/Start Pairing/i)).not.toBeInTheDocument();

    const continueButton = screen.getByRole('button', { name: 'Continue →' }) as HTMLButtonElement;
    expect(continueButton.disabled).toBe(true);

    fireEvent.change(screen.getByLabelText('Channel Bot Token'), { target: { value: 'telegram-openclaw-token' } });

    await waitFor(() => expect(continueButton.disabled).toBe(false));
  });
});
