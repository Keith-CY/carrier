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
        return new Response(JSON.stringify({
          features: {
            remoteControlPlaneEnabled: true,
            remoteChatEnabled: true,
            providerBindingEnabled: true,
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
      if (url.endsWith('/healthz')) {
        return new Response(JSON.stringify({ status: 'ok' }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      return new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json' } });
    }) as typeof fetch;
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  test('redirects root to /home', async () => {
    const router = renderAt('/');
    await waitFor(() => expect(router.state.location.pathname).toBe('/home'));
  });

  test('redirects unknown routes to /home', async () => {
    const router = renderAt('/does-not-exist');
    await waitFor(() => expect(router.state.location.pathname).toBe('/home'));
  });

  test('includes the new guided hub routes', () => {
    const rootRoute = routeObjects[0];
    const childPaths = Array.isArray(rootRoute.children) ? rootRoute.children.map((route) => route.path) : [];
    expect(childPaths).toContain('onboarding');
    expect(childPaths).toContain('welcome');
    expect(childPaths).toContain('setup');
    expect(childPaths).toContain('provider');
    expect(childPaths).toContain('install');
    expect(childPaths).toContain('complete');
    expect(childPaths).toContain('add/:agentId');
    expect(childPaths).toContain('home');
    expect(childPaths).toContain('dashboard');
    expect(childPaths).toContain('quick-entry');
    expect(childPaths).toContain('projects');
    expect(childPaths).toContain('projects/:projectId');
    expect(childPaths).toContain('agents');
    expect(childPaths).toContain('agents/:agentId');
    expect(childPaths).toContain('hosts');
    expect(childPaths).toContain('providers');
    expect(childPaths).toContain('remote-chat');
    expect(childPaths).toContain('executions');
    expect(childPaths).toContain('executions/:executionId');
    expect(childPaths).toContain('activity');
    expect(childPaths).toContain('settings');
  });
});
