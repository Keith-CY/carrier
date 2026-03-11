import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider, createMemoryRouter } from 'react-router-dom';
import { routeObjects } from './routes';
import { SessionProvider } from './session';

function renderAt(pathname: string) {
  const router = createMemoryRouter(routeObjects, { initialEntries: [pathname] });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <SessionProvider>
        <RouterProvider router={router} />
      </SessionProvider>
    </QueryClientProvider>,
  );
  return router;
}

describe('router redirects', () => {
  beforeEach(() => {
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url);
      if (url.endsWith('/api/v1/features')) {
        return new Response(JSON.stringify({ features: {}, authz: {} }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      if (url.endsWith('/healthz')) {
        return new Response(JSON.stringify({ status: 'ok' }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      return new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json' } });
    }) as typeof fetch;
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  test('redirects root to /welcome', async () => {
    const router = renderAt('/');
    await waitFor(() => expect(router.state.location.pathname).toBe('/welcome'));
  });

  test('redirects unknown routes to /welcome', async () => {
    const router = renderAt('/does-not-exist');
    await waitFor(() => expect(router.state.location.pathname).toBe('/welcome'));
  });
});
