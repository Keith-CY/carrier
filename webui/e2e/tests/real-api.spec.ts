import { test, expect } from '@playwright/test';

/**
 * Real API tests — these require a running daemon and gateway.
 * Skipped by default; set SKIP_REAL_API=0 or unset it to run.
 * 
 * To run locally:
 *   cd daemon && go build -tags webui ./cmd/agentd
 *   ./agentd &
 *   SKIP_REAL_API=0 npx playwright test real-api
 */
test.describe('real API', () => {
  const skip = process.env.SKIP_REAL_API !== '0';

  test.skip(skip, 'SKIP_REAL_API is set; skipping real API tests');

  const baseURL = process.env.DAEMON_URL || 'http://localhost:9090';

  test('healthz returns JSON', async ({ request }) => {
    const res = await request.get(`${baseURL}/healthz`);
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    expect(body).toHaveProperty('status');
  });

  test('command endpoint responds', async ({ request }) => {
    const res = await request.post(`${baseURL}/command`, {
      data: { command: 'list' },
    });
    // Even if unauthorized, should get a structured response
    expect([200, 401, 403]).toContain(res.status());
  });

  test('setup endpoint responds', async ({ request }) => {
    const res = await request.get(`${baseURL}/api/v1/setup`);
    expect([200, 401, 403, 404]).toContain(res.status());
  });
});
