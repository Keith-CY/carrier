import { expect, test } from '@playwright/test';
import { loginWithToken, mockAPIs } from './helpers';

/**
 * E2E test for the SSH passphrase + OpenClaw sync flow via ClawPal UI.
 *
 * Tests the full UI flow: add a remote host with private_key auth,
 * verify it appears in the list, open manage panel, and trigger sync.
 * Uses mocked API routes to simulate the backend responses.
 */
test.describe('SSH Passphrase Sync E2E', () => {
  test('add host with private_key auth, connect, and sync config', async ({ page }) => {
    await mockAPIs(page);

    // ── State for mocked API handlers ──
    const hosts: Array<Record<string, unknown>> = [];
    let checkCalls = 0;
    let instancesCalls = 0;
    let syncCalls = 0;

    // Remote hosts: create + list
    await page.route('**/api/v1/remote/hosts', async (route) => {
      const req = route.request();
      if (req.method() === 'GET') {
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ result: 'ok', hosts }),
        });
      }
      if (req.method() === 'POST') {
        const body = req.postDataJSON() as Record<string, unknown>;
        const host = {
          id: `host-${hosts.length + 1}`,
          ...body,
          lastHealth: 'unknown',
        };
        hosts.push(host);
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ result: 'ok', host }),
        });
      }
      return route.fallback();
    });

    // Health check
    await page.route('**/api/v1/remote/hosts/*/check', async (route) => {
      checkCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-ssh-check-1',
          result: 'ok',
          check: { sshOk: true, openclawFound: true, gatewayHealthy: true },
        }),
      });
    });

    // Host PATCH/DELETE
    await page.route('**/api/v1/remote/hosts/*', async (route) => {
      const req = route.request();
      if (req.method() === 'PATCH') {
        const body = req.postDataJSON() as Record<string, unknown>;
        const path = new URL(req.url()).pathname;
        const hostID = path.split('/').pop() || '';
        const idx = hosts.findIndex((h) => String(h.id) === hostID);
        if (idx >= 0) {
          hosts[idx] = { ...hosts[idx], ...body, id: hostID };
        }
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ result: 'ok', host: hosts[idx >= 0 ? idx : 0] }),
        });
      }
      return route.fallback();
    });

    // Instances list
    await page.route('**/api/v1/remote/hosts/*/instances', async (route) => {
      instancesCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-instances-1',
          result: 'ok',
          instances: [
            {
              id: 'host-1:main',
              agentId: 'main',
              runtimeState: 'running',
              health: 'healthy',
            },
          ],
        }),
      });
    });

    // Sync
    await page.route('**/api/v1/remote/hosts/*/instances/*/sync', async (route) => {
      syncCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-sync-1',
          result: 'ok',
          sync: {
            hostId: 'host-1',
            agentId: 'main',
            mode: 'pull_validate_push',
            driftState: 'in_sync',
          },
          steps: [{ command: 'sync pull', exitCode: 0 }],
        }),
      });
    });

    // Instance status
    await page.route('**/api/v1/remote/hosts/*/instances/*/status', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-status-1',
          result: 'ok',
          instance: { id: 'host-1:main', agentId: 'main', runtimeState: 'running', health: 'healthy' },
        }),
      }),
    );

    // ── Step 1: Navigate to servers page ──
    await loginWithToken(page, '/#/servers');

    // ── Step 2: Fill in the "Add / Update Server" form ──
    await page.selectOption('#server-auth-mode', 'private_key');
    await page.fill('#server-name', 'e2e-docker-ssh');
    await page.fill('#server-host', '127.0.0.1');
    await page.fill('#server-port', '2222');
    await page.fill('#server-user', 'testuser');
    await page.fill('#server-key-path', '~/.ssh/id_ed25519');

    // ── Step 3: Click "Save Host" ──
    await page.click('#server-save');

    // ── Step 4: Verify the host appears in the server list ──
    await expect(
      page.locator('#servers-list .agent-card h4', { hasText: 'e2e-docker-ssh' }),
    ).toBeVisible();

    // Verify the saved host data is correct
    expect(hosts.length).toBe(1);
    expect(hosts[0].name).toBe('e2e-docker-ssh');
    expect(hosts[0].host).toBe('127.0.0.1');
    expect(hosts[0].user).toBe('testuser');
    expect(hosts[0].authMode).toBe('private_key');
    expect(hosts[0].keyPath).toBe('~/.ssh/id_ed25519');

    // ── Step 5: Run health check ──
    await page.locator('#servers-list .agent-card').first().locator('button', { hasText: 'Check' }).click();
    await expect.poll(() => checkCalls).toBe(1);
    await expect(
      page.locator('#servers-list .agent-card').first().locator('.instance-meta'),
    ).toContainText('last op: host_check (ok)');

    // ── Step 6: Click "Manage" to open manage panel ──
    await page.locator('#servers-list .agent-card').first().locator('button', { hasText: 'Manage' }).click();
    await expect(page.locator('#server-manage-card')).toBeVisible();
    await expect(page.locator('#server-manage-host-label')).toContainText('host-1');

    // ── Step 7: Load instances ──
    await page.click('#server-manage-load-instances');
    await expect.poll(() => instancesCalls).toBe(1);
    await expect(page.locator('#server-manage-instances')).toContainText('main (runtime=running, health=healthy)');

    // ── Step 8: Click "Sync" and verify result ──
    await page.click('#server-manage-sync-instance');
    await expect.poll(() => syncCalls).toBe(1);
    await expect(page.locator('#server-manage-instance-status-out')).toContainText('drift state: in_sync');
  });

  test('add host with ssh_config auth mode', async ({ page }) => {
    await mockAPIs(page);

    const hosts: Array<Record<string, unknown>> = [];

    await page.route('**/api/v1/remote/hosts', async (route) => {
      const req = route.request();
      if (req.method() === 'GET') {
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ result: 'ok', hosts }),
        });
      }
      if (req.method() === 'POST') {
        const body = req.postDataJSON() as Record<string, unknown>;
        const host = { id: `host-${hosts.length + 1}`, ...body, lastHealth: 'unknown' };
        hosts.push(host);
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ result: 'ok', host }),
        });
      }
      return route.fallback();
    });

    await page.route('**/api/v1/remote/hosts/*', async (route) => route.fallback());

    await loginWithToken(page, '/#/servers');

    // Switch to ssh_config mode
    await page.selectOption('#server-auth-mode', 'ssh_config');
    await page.fill('#server-name', 'config-host');
    await page.fill('#server-host', '10.0.0.5');
    await page.fill('#server-ssh-config-host', 'my-prod-server');
    await page.click('#server-save');

    await expect(
      page.locator('#servers-list .agent-card h4', { hasText: 'config-host' }),
    ).toBeVisible();

    expect(hosts.length).toBe(1);
    expect(hosts[0].authMode).toBe('ssh_config');
    expect(hosts[0].sshConfigHost).toBe('my-prod-server');
  });

  test('sync failure shows error in UI', async ({ page }) => {
    await mockAPIs(page);

    const hosts = [
      {
        id: 'host-fail',
        name: 'failing-host',
        host: '10.0.0.99',
        user: 'ubuntu',
        authMode: 'private_key',
        keyPath: '~/.ssh/id_ed25519',
        runtimeMode: 'on_demand',
        lastHealth: 'unknown',
      },
    ];

    await page.route('**/api/v1/remote/hosts', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ result: 'ok', hosts }),
      }),
    );

    await page.route('**/api/v1/remote/hosts/*/check', async (route) =>
      route.fulfill({
        status: 502,
        contentType: 'application/json',
        body: JSON.stringify({
          result: 'error',
          message: 'ssh: connect to host 10.0.0.99 port 22: Connection timed out',
        }),
      }),
    );

    await page.route('**/api/v1/remote/hosts/*', async (route) => route.fallback());

    await page.route('**/api/v1/remote/hosts/*/instances', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-inst-fail',
          result: 'ok',
          instances: [{ id: 'host-fail:main', agentId: 'main', runtimeState: 'stopped', health: 'unhealthy' }],
        }),
      }),
    );

    await page.route('**/api/v1/remote/hosts/*/instances/*/sync', async (route) =>
      route.fulfill({
        status: 502,
        contentType: 'application/json',
        body: JSON.stringify({
          result: 'error',
          message: 'ssh connection failed: timeout',
        }),
      }),
    );

    await page.route('**/api/v1/remote/hosts/*/instances/*/status', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-status-fail',
          result: 'ok',
          instance: { id: 'host-fail:main', agentId: 'main', runtimeState: 'stopped', health: 'unhealthy' },
        }),
      }),
    );

    await loginWithToken(page, '/#/servers');

    // Verify host is listed
    await expect(
      page.locator('#servers-list .agent-card h4', { hasText: 'failing-host' }),
    ).toBeVisible();

    // Health check should fail
    await page.locator('#servers-list .agent-card').first().locator('button', { hasText: 'Check' }).click();
    await expect(page.locator('#servers-msg')).toContainText('Health check failed');

    // Open manage panel and try sync
    await page.locator('#servers-list .agent-card').first().locator('button', { hasText: 'Manage' }).click();
    await expect(page.locator('#server-manage-card')).toBeVisible();

    await page.click('#server-manage-load-instances');
    await expect(page.locator('#server-manage-instances')).toContainText('main');

    await page.click('#server-manage-sync-instance');
    await expect(page.locator('#server-manage-msg')).toContainText('ssh connection failed');
  });
});
