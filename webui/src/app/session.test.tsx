import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { SessionProvider, useSession } from './session';

function SessionProbe() {
  const { authenticated, loginError, health, login, logout } = useSession();

  return (
    <div>
      <div data-testid="authenticated">{authenticated ? 'yes' : 'no'}</div>
      <div data-testid="login-error">{loginError}</div>
      <div data-testid="health">{health.text}</div>
      <button type="button" onClick={() => void login('test-token')}>Login</button>
      <button type="button" onClick={() => logout()}>Logout</button>
    </div>
  );
}

function renderSessionProbe() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });

  render(
    <QueryClientProvider client={queryClient}>
      <SessionProvider>
        <SessionProbe />
      </SessionProvider>
    </QueryClientProvider>,
  );
}

describe('SessionProvider', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  test('starts unauthenticated without a stored token', () => {
    renderSessionProbe();

    expect(screen.getByTestId('authenticated')).toHaveTextContent('no');
    expect(screen.getByTestId('health')).toHaveTextContent('auth required');
  });

  test('logs in with a valid token and stores it', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ status: 'ok' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })) as typeof fetch;

    renderSessionProbe();
    fireEvent.click(screen.getByText('Login'));

    await waitFor(() => expect(screen.getByTestId('authenticated')).toHaveTextContent('yes'));
    expect(localStorage.getItem('carrier_token')).toBe('test-token');
    expect(screen.getByTestId('health')).toHaveTextContent('online');
  });

  test('clears token and returns to unauthenticated state on auth-expired event', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ status: 'ok' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })) as typeof fetch;

    renderSessionProbe();
    fireEvent.click(screen.getByText('Login'));
    await waitFor(() => expect(screen.getByTestId('authenticated')).toHaveTextContent('yes'));

    window.dispatchEvent(new CustomEvent('carrier:auth-expired'));

    await waitFor(() => expect(screen.getByTestId('authenticated')).toHaveTextContent('no'));
    expect(localStorage.getItem('carrier_token')).toBeNull();
    expect(screen.getByTestId('health')).toHaveTextContent('auth required');
  });
});
