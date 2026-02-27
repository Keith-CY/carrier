import { Page } from '@playwright/test';

export const TEST_TOKEN = 'test-token-valid';
export const MOCK_LOG_STREAM_LINES = [
  '{"time":"2026-02-22T10:00:00.000Z","level":"INFO","message":"agent started"}',
  '[DEBUG] worker heartbeat',
  '2026-02-22T10:00:01.000Z WARN queue depth high',
  '2026-02-22T10:00:02.000Z ERROR request failed',
];

export const MOCK_AGENTS = [
  { id: 'agent-alpha', name: 'agent-alpha', runtime: 'running' },
  { id: 'agent-beta', name: 'agent-beta', runtime: 'error' },
  { id: 'agent-gamma', name: 'agent-gamma', runtime: 'stopped' },
];

export const MOCK_INSTANCES = MOCK_AGENTS.map((agent, idx) => ({
  id: `instance-${idx + 1}`,
  agent_id: agent.id,
  runtime_state: agent.runtime,
  provider: 'openai',
  channel: 'telegram',
}));

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

  await page.route('**/api/v1/instances', (route) => {
    if (route.request().method() === 'GET') {
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_INSTANCES),
      });
    }
    return route.continue();
  });

  await page.route('**/api/v1/instances/*/*', (route) =>
    route.fulfill({ status: 200, contentType: 'application/json', body: '{"ok":true}' }),
  );

  await page.route('**/api/v1/agents/*/install', (route) =>
    route.fulfill({ status: 200, contentType: 'application/json', body: '{"ok":true}' }),
  );

  await page.route('**/api/v1/agents/*/start', (route) =>
    route.fulfill({ status: 200, contentType: 'application/json', body: '{"ok":true}' }),
  );

  await page.route('**/api/v1/onboard', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        onboard: {
          channel: 'skip',
          providerId: 'openai',
          webuiOnly: true,
          pairRequired: false,
        },
      }),
    }),
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

  await page.route('**/api/v1/providers', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        by_category: {
          'API Key': [
            { id: 'openai', name: 'OpenAI', auth_mode: 'api_key', env_var: 'OPENAI_API_KEY', example_model: 'openai/gpt-4o', description: 'OpenAI API' },
          ],
        },
      }),
    }),
  );

  await page.route('**/api/v1/features', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        features: {
          remoteControlPlaneEnabled: true,
          remoteChatEnabled: true,
          providerBindingEnabled: true,
        },
      }),
    }),
  );

  await page.route('**/api/v1/remote/metrics', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        metrics: {
          totals: { total: 0, success: 0, failure: 0, successRate: 0, avgLatencyMs: 0, minLatencyMs: 0, maxLatencyMs: 0, latencyTotalMs: 0 },
          repair: { triggered: 0, success: 0, failure: 0, successRate: 0 },
          chatStream: { total: 0, failure: 0, failureRate: 0 },
          rollout: { state: 'healthy', canPromote: true, reasons: [] },
          operations: {},
          timestamp: '2026-02-25T00:00:00Z',
        },
      }),
    }),
  );

  // SSE log stream — return a small SSE payload then close
  await page.route('**/api/v1/logs/stream*', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      body: MOCK_LOG_STREAM_LINES.map((line) => `data: ${line}\n\n`).join(''),
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
