import { Page } from '@playwright/test';

export const TEST_TOKEN = 'test-token-valid';

export const MOCK_AGENTS = [
  { id: 'agent-alpha', name: 'agent-alpha', runtime: 'running' },
  { id: 'agent-beta', name: 'agent-beta', runtime: 'error' },
  { id: 'agent-gamma', name: 'agent-gamma', runtime: 'stopped' },
];

/** Set up standard API mocks so the app works without a real daemon. */
export async function mockAPIs(page: Page, opts?: { healthOk?: boolean }) {
  const healthOk = opts?.healthOk ?? true;

  await page.route('**/healthz', (route) =>
    route.fulfill({
      status: healthOk ? 200 : 401,
      contentType: 'application/json',
      body: JSON.stringify(healthOk ? { status: 'ok' } : { status: 'error' }),
    }),
  );

  await page.route('**/api/v1/agents', (route) => {
    if (route.request().method() === 'GET') {
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_AGENTS),
      });
    }
    return route.continue();
  });

  await page.route('**/api/v1/agents/*/install', (route) =>
    route.fulfill({ status: 200, contentType: 'application/json', body: '{"ok":true}' }),
  );

  await page.route('**/api/v1/agents/*/start', (route) =>
    route.fulfill({ status: 200, contentType: 'application/json', body: '{"ok":true}' }),
  );

  await page.route('**/api/v1/agents/*/stop', (route) =>
    route.fulfill({ status: 200, contentType: 'application/json', body: '{"ok":true}' }),
  );

  await page.route('**/api/v1/agents/*/status', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ id: 'agent-alpha', runtime: 'running', uptime: '1h' }),
    }),
  );

  await page.route('**/api/v1/agents/*/logs', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ lines: ['[INFO] agent started', '[INFO] listening on :8080'] }),
    }),
  );

  // SSE log stream — return a small SSE payload then close
  await page.route('**/api/v1/logs/stream*', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      body: 'data: [INFO] log line 1\n\ndata: [INFO] log line 2\n\n',
    }),
  );
}

/** Login by setting token in localStorage and navigating. */
export async function loginWithToken(page: Page, url = '/') {
  await page.addInitScript((token: string) => {
    localStorage.setItem('carrier_token', token);
  }, TEST_TOKEN);
  await page.goto(url);
}
