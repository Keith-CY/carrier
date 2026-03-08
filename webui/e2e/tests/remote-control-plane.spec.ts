import { expect, test } from '@playwright/test';
import { loginWithToken, mockAPIs } from './helpers';

test.describe('Remote Control Plane Views', () => {
  test('profiles binding controls are disabled when provider binding feature is off', async ({ page }) => {
    await mockAPIs(page);
    await page.route('**/api/v1/features', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          result: 'ok',
          features: {
            remoteControlPlaneEnabled: true,
            remoteChatEnabled: true,
            providerBindingEnabled: false,
          },
        }),
      }),
    );
    await page.route('**/api/v1/remote/hosts', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ result: 'ok', hosts: [{ id: 'host-1', name: 'prod-host-1' }] }),
      }),
    );
    await page.route('**/api/v1/provider-profiles', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          result: 'ok',
          profiles: [{ id: 'profile-1', name: 'openai-gpt5', provider: 'openai', model: 'gpt-5' }],
        }),
      }),
    );
    await page.route('**/api/v1/provider-bindings', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ result: 'ok', bindings: [] }),
      }),
    );

    await loginWithToken(page, '/#/profiles');

    await expect(page.locator('#binding-save')).toBeDisabled();
    await expect(page.locator('#binding-profile-id')).toBeDisabled();
    await expect(page.locator('#profiles-msg')).toContainText('Provider binding is disabled by feature flag.');
  });

  test('feature flags hide remote control routes when disabled', async ({ page }) => {
    await mockAPIs(page);
    await page.route('**/api/v1/features', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          result: 'ok',
          features: {
            remoteControlPlaneEnabled: false,
            remoteChatEnabled: false,
            providerBindingEnabled: false,
          },
        }),
      }),
    );

    await loginWithToken(page, '/#/servers');

    await expect.poll(() => page.url()).toContain('#/dashboard');
    await expect(page.locator('#view-dashboard')).toBeVisible();
    await expect(page.locator('.nav-link[data-route="workers"]')).toBeHidden();
    await expect(page.locator('.nav-link[data-route="servers"]')).toBeHidden();
    await expect(page.locator('.nav-link[data-route="profiles"]')).toBeHidden();
    await expect(page.locator('.nav-link[data-route="remote-chat"]')).toBeHidden();
    await expect(page.locator('.nav-link[data-route="remote-observability"]')).toBeHidden();

    await page.goto('/#/remote-observability');
    await expect.poll(() => page.url()).toContain('#/dashboard');
  });

  test('servers page supports create/check/delete flow', async ({ page }) => {
    await mockAPIs(page);

    const hosts: Array<Record<string, unknown>> = [];
    let patchCalls = 0;
    let lastPatchBody: Record<string, unknown> | null = null;
    let healthCheckCalls = 0;
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

    await page.route('**/api/v1/remote/hosts/*/check', async (route) =>
      {
        healthCheckCalls += 1;
        if (healthCheckCalls > 1) {
          return route.fulfill({
            status: 502,
            contentType: 'application/json',
            body: JSON.stringify({
              result: 'error',
              message: 'ssh timeout',
            }),
          });
        }
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            requestId: 'req-host-check-1',
            result: 'ok',
            check: { sshOk: true, openclawFound: true, gatewayHealthy: true },
          }),
        });
      },
    );

    await page.route('**/api/v1/remote/hosts/*', async (route) => {
      const req = route.request();
      if (req.method() === 'PATCH') {
        patchCalls += 1;
        const body = req.postDataJSON() as Record<string, unknown>;
        lastPatchBody = body;
        const path = new URL(req.url()).pathname;
        const hostID = path.split('/').pop() || '';
        const idx = hosts.findIndex((h) => String(h.id) === hostID);
        if (idx >= 0) {
          hosts[idx] = {
            ...hosts[idx],
            ...body,
            id: hostID,
          };
        }
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            result: 'ok',
            host: hosts[idx >= 0 ? idx : 0],
          }),
        });
      }
      if (req.method() !== 'DELETE') return route.fallback();
      const path = new URL(req.url()).pathname;
      const hostID = path.split('/').pop() || '';
      const idx = hosts.findIndex((h) => String(h.id) === hostID);
      if (idx >= 0) hosts.splice(idx, 1);
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ result: 'ok', deleted: true }),
      });
    });

    await loginWithToken(page, '/#/servers');

    await page.fill('#server-name', 'prod-eu-1');
    await page.fill('#server-host', '10.0.0.12');
    await page.fill('#server-user', 'ubuntu');
    await page.fill('#server-key-path', '~/.ssh/id_ed25519');
    await page.click('#server-save');

    await expect(page.locator('#servers-list .agent-card h4', { hasText: 'prod-eu-1' })).toBeVisible();

    await page.locator('#servers-list .agent-card').first().locator('button', { hasText: 'Edit' }).click();
    await expect(page.locator('#server-editor-state')).toContainText('Editing host');
    await expect(page.locator('#server-save')).toContainText('Update Host');
    await page.fill('#server-name', 'prod-eu-1-updated');
    await page.click('#server-save');
    await expect.poll(() => patchCalls).toBe(1);
    expect(lastPatchBody?.name).toBe('prod-eu-1-updated');
    await expect(page.locator('#servers-list .agent-card h4', { hasText: 'prod-eu-1-updated' })).toBeVisible();
    await expect(page.locator('#server-save')).toContainText('Save Host');

    await page.locator('#servers-list .agent-card').first().locator('button', { hasText: 'Check' }).click();
    await expect.poll(() => healthCheckCalls).toBe(1);
    await expect(page.locator('#servers-list .agent-card').first().locator('.instance-meta')).toContainText('last op: host_check (ok)');
    await expect(page.locator('#servers-list .agent-card').first().locator('.instance-meta')).toContainText('requestId: req-host-check-1');
    await expect(page.locator('#servers-list .agent-card').first().locator('.instance-meta')).toContainText(/updated at:\s+\d{4}-\d{2}-\d{2}T/);

    await page.locator('#servers-list .agent-card').first().locator('button', { hasText: 'Check' }).click();
    await expect.poll(() => healthCheckCalls).toBe(2);
    await expect(page.locator('#servers-msg')).toContainText('Health check failed: ssh timeout');
    await expect(page.locator('#servers-list .agent-card').first().locator('.instance-meta')).toContainText('last op: host_check (error)');
    await expect(page.locator('#servers-list .agent-card').first().locator('.instance-meta')).toContainText('last error: ssh timeout');

    page.once('dialog', (dialog) => dialog.accept());
    await page.locator('#servers-list .agent-card').first().locator('button', { hasText: 'Delete' }).click();
    await expect.poll(() => hosts.length).toBe(0);
    await expect(page.locator('#servers-list .card')).toContainText('No remote servers configured.');
  });

  test('servers page supports config, sessions, and memory management actions', async ({ page }) => {
    await mockAPIs(page);

    const hosts = [{ id: 'host-1', name: 'prod-host-1', host: '10.0.0.9', user: 'ubuntu', authMode: 'private_key', runtimeMode: 'on_demand' }];
    let lastConfigPatchBody: Record<string, unknown> | null = null;
    let archiveCalls = 0;
    let deleteCalls = 0;
    let installCalls = 0;
    let repairCalls = 0;
    let statusCalls = 0;
    let logsCalls = 0;
    let syncCalls = 0;
    let syncStatusCalls = 0;
    let diagnoseCalls = 0;
    let reconcileCalls = 0;
    let rollbackCalls = 0;
    let codeagentInstallCalls = 0;
    let codeagentHealthCalls = 0;
    let codeagentVersionCalls = 0;
    let codeagentRunCalls = 0;

    await page.route('**/api/v1/remote/hosts', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ result: 'ok', hosts }),
      }),
    );

    await page.route('**/api/v1/remote/hosts/*/config', async (route) => {
      const req = route.request();
      if (req.method() === 'GET') {
        await new Promise((resolve) => setTimeout(resolve, 180));
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            requestId: 'req-config-get-1',
            result: 'ok',
            config: {
              agents: {
                defaults: {
                  provider: 'openai',
                  model: 'gpt-4.1',
                },
              },
            },
          }),
        });
      }
      if (req.method() === 'PATCH') {
        lastConfigPatchBody = req.postDataJSON() as Record<string, unknown>;
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            requestId: 'req-config-patch-1',
            result: 'ok',
            config: lastConfigPatchBody,
          }),
        });
      }
      return route.fallback();
    });

    await page.route('**/api/v1/remote/hosts/*/instances', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-instances-list-1',
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
      }),
    );

    await page.route('**/api/v1/remote/hosts/*/instances/*/status', async (route) => {
      statusCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-instance-status-1',
          result: 'ok',
          instance: {
            id: 'host-1:main',
            agentId: 'main',
            runtimeState: 'running',
            health: 'healthy',
          },
          steps: [{ command: 'openclaw status', exitCode: 0 }],
        }),
      });
    });

    await page.route('**/api/v1/remote/hosts/*/instances/*/install*', async (route) => {
      installCalls += 1;
      await new Promise((resolve) => setTimeout(resolve, 250));
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-install-1',
          result: 'ok',
          install: {
            hostId: 'host-1',
            agentId: 'main',
            installed: true,
          },
        }),
      });
    });

    await page.route('**/api/v1/remote/hosts/*/instances/*/sync/status', async (route) => {
      syncStatusCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-sync-status-1',
          result: 'ok',
          status: {
            hostId: 'host-1',
            agentId: 'main',
            syncMode: 'pull_validate_push',
            driftState: 'in_sync',
            lastSyncStatus: 'success',
          },
        }),
      });
    });

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

    await page.route('**/api/v1/remote/hosts/*/instances/*/diagnose', async (route) => {
      diagnoseCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-diagnose-1',
          result: 'ok',
          diagnose: {
            hostId: 'host-1',
            agentId: 'main',
            result: 'healthy',
            driftState: 'in_sync',
          },
          steps: [{ command: 'diagnose drift', exitCode: 0 }],
        }),
      });
    });

    await page.route('**/api/v1/remote/hosts/*/instances/*/reconcile', async (route) => {
      reconcileCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-reconcile-1',
          result: 'ok',
          reconcile: {
            hostId: 'host-1',
            agentId: 'main',
            reconciled: true,
            driftState: 'in_sync',
          },
          steps: [{ command: 'apply config', exitCode: 0 }],
        }),
      });
    });

    await page.route('**/api/v1/remote/hosts/*/instances/*/rollback', async (route) => {
      rollbackCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-rollback-1',
          result: 'ok',
          rollback: {
            hostId: 'host-1',
            agentId: 'main',
            rolledBack: true,
            fromCommit: 'abc123',
            newCommit: 'def456',
          },
          steps: [{ command: 'rollback profile', exitCode: 0 }],
        }),
      });
    });

    await page.route('**/api/v1/remote/hosts/*/instances/*/codeagent/install', async (route) => {
      codeagentInstallCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-codeagent-install-1',
          result: 'ok',
          install: {
            backend: 'codex',
            workspaceRoot: '/workspace',
            installed: true,
            version: 'codex 1.0.0',
          },
        }),
      });
    });

    await page.route('**/api/v1/remote/hosts/*/instances/*/codeagent/health*', async (route) => {
      codeagentHealthCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-codeagent-health-1',
          result: 'ok',
          health: {
            backend: 'codex',
            healthy: true,
          },
        }),
      });
    });

    await page.route('**/api/v1/remote/hosts/*/instances/*/codeagent/version*', async (route) => {
      codeagentVersionCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-codeagent-version-1',
          result: 'ok',
          version: {
            backend: 'codex',
            value: 'codex 1.0.0',
          },
        }),
      });
    });

    await page.route('**/api/v1/remote/hosts/*/instances/*/codeagent/run', async (route) => {
      codeagentRunCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-codeagent-run-1',
          result: 'ok',
          run: {
            backend: 'codex',
            result: {
              ok: true,
              exit_code: 0,
              stdout: 'hello from codeagent',
              policy_decision: 'allow',
            },
          },
        }),
      });
    });

    await page.route('**/api/v1/remote/hosts/*/instances/*/repair', async (route) => {
      repairCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-repair-1',
          result: 'ok',
          repair: {
            hostId: 'host-1',
            agentId: 'main',
            repaired: true,
            gatewayHealthy: true,
          },
        }),
      });
    });

    await page.route('**/api/v1/remote/hosts/*/instances/*/logs*', async (route) => {
      logsCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-logs-1',
          result: 'ok',
          logs: '--- gateway.log ---\nready\n',
        }),
      });
    });

    await page.route('**/api/v1/remote/hosts/*/sessions*', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          result: 'ok',
          sessions: [
            {
              sessionId: 'sess-1',
              kind: 'sessions',
              sizeBytes: 128,
              modifiedAt: 1730101000,
              path: '/home/ubuntu/.openclaw/agents/main/sessions/sess-1.jsonl',
            },
          ],
        }),
      }),
    );

    await page.route('**/api/v1/remote/hosts/*/sessions/*/archive*', async (route) => {
      archiveCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ result: 'ok', action: 'archive' }),
      });
    });

    await page.route('**/api/v1/remote/hosts/*/sessions/*/delete*', async (route) => {
      deleteCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ result: 'ok', action: 'delete' }),
      });
    });

    await page.route('**/api/v1/remote/hosts/*/memory*', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          result: 'ok',
          memory: [
            {
              path: 'facts/system.md',
              sizeBytes: 64,
              modifiedAt: 1730102000,
            },
          ],
        }),
      }),
    );

    await loginWithToken(page, '/#/servers');

    await expect(page.locator('#servers-list .agent-card h4', { hasText: 'prod-host-1' })).toBeVisible();
    await page.locator('#servers-list .agent-card').first().locator('button', { hasText: 'Manage' }).click();
    await expect(page.locator('#server-manage-card')).toBeVisible();
    await expect(page.locator('#server-manage-host-label')).toContainText('host-1');

    await page.click('#server-manage-load-instances');
    await expect(page.locator('#server-manage-instances')).toContainText('main (runtime=running, health=healthy)');

    await page.click('#server-manage-instance-status');
    await expect.poll(() => statusCalls).toBe(1);
    await expect(page.locator('#server-manage-instance-status-out')).toContainText('runtime: running');

    const installBtn = page.locator('#server-manage-install-instance');
    await installBtn.click();
    await expect(installBtn).toBeDisabled();
    await expect(page.locator('#server-manage-msg')).toContainText('Install completed');
    await expect(installBtn).toBeEnabled();
    await expect(page.locator('#server-manage-instance-status-out')).toContainText('Install');

    await page.click('#server-manage-repair-instance');
    await expect.poll(() => repairCalls).toBe(1);
    await expect(page.locator('#server-manage-instance-status-out')).toContainText('repair status: repaired');

    await page.fill('#server-manage-log-tail', '50');
    await page.click('#server-manage-load-logs');
    await expect.poll(() => logsCalls).toBe(1);
    await expect(page.locator('#server-manage-logs')).toContainText('gateway.log');
    await expect(page.locator('#server-manage-logs')).toContainText('ready');
    await expect(page.locator('#server-manage-op-meta')).toContainText('operation=instance_logs');
    await expect(page.locator('#server-manage-op-meta')).toContainText('requestId=req-logs-1');
    await expect(page.locator('#server-manage-op-meta')).toContainText('duration=');

    await page.click('#server-manage-sync-instance');
    await expect.poll(() => syncCalls).toBe(1);
    await expect(page.locator('#server-manage-instance-status-out')).toContainText('drift state: in_sync');

    await page.click('#server-manage-sync-status');
    await expect.poll(() => syncStatusCalls).toBe(1);
    await expect(page.locator('#server-manage-instance-status-out')).toContainText('sync status: success');

    await page.click('#server-manage-diagnose-instance');
    await expect.poll(() => diagnoseCalls).toBe(1);
    await expect(page.locator('#server-manage-instance-status-out')).toContainText('diagnose result: healthy');

    await page.click('#server-manage-reconcile-instance');
    await expect.poll(() => reconcileCalls).toBe(1);
    await expect(page.locator('#server-manage-instance-status-out')).toContainText('reconciled: true');

    await page.click('#server-manage-rollback-instance');
    await expect.poll(() => rollbackCalls).toBe(1);
    await expect(page.locator('#server-manage-instance-status-out')).toContainText('rolled back: true');

    await page.fill('#server-manage-codeagent-command', 'echo hello');
    await page.click('#server-manage-codeagent-install');
    await expect.poll(() => codeagentInstallCalls).toBe(1);
    await expect(page.locator('#server-manage-instance-status-out')).toContainText('backend: codex');

    await page.click('#server-manage-codeagent-health');
    await expect.poll(() => codeagentHealthCalls).toBe(1);
    await expect(page.locator('#server-manage-instance-status-out')).toContainText('healthy: true');

    await page.click('#server-manage-codeagent-version');
    await expect.poll(() => codeagentVersionCalls).toBe(1);
    await expect(page.locator('#server-manage-instance-status-out')).toContainText('backend: codex');

    await page.click('#server-manage-codeagent-run');
    await expect.poll(() => codeagentRunCalls).toBe(1);
    await expect(page.locator('#server-manage-logs')).toContainText('hello from codeagent');

    const loadConfigBtn = page.locator('#server-manage-load-config');
    await loadConfigBtn.click();
    await expect(page.locator('#server-manage-msg')).toContainText('Loading config');
    await expect(loadConfigBtn).toBeDisabled();
    await expect(loadConfigBtn).toBeEnabled();
    await expect(page.locator('#server-manage-config')).toHaveValue(/"provider":\s*"openai"/);
    await expect(page.locator('#server-manage-op-meta')).toContainText('operation=load_config');
    await expect(page.locator('#server-manage-op-meta')).toContainText('requestId=req-config-get-1');
    await expect(page.locator('#servers-list .agent-card').first().locator('.instance-meta')).toContainText('last op: load_config (ok)');
    await expect(page.locator('#servers-list .agent-card').first().locator('.instance-meta')).toContainText('requestId: req-config-get-1');

    await page.fill('#server-manage-config', JSON.stringify({
      agents: {
        defaults: {
          model: 'gpt-5',
        },
      },
    }));
    await page.click('#server-manage-apply-config');
    expect(lastConfigPatchBody).toBeTruthy();
    expect((lastConfigPatchBody?.agents as Record<string, unknown>)?.defaults).toBeTruthy();

    await page.click('#server-manage-load-sessions');
    await expect(page.locator('#server-manage-sessions')).toContainText('sess-1');

    await page.fill('#server-manage-session-id', 'sess-1');
    await page.click('#server-manage-archive-session');
    await expect.poll(() => archiveCalls).toBe(1);
    await page.click('#server-manage-delete-session');
    await expect.poll(() => deleteCalls).toBe(1);

    await page.click('#server-manage-load-memory');
    await expect(page.locator('#server-manage-memory')).toContainText('facts/system.md');
  });

  test('profiles page supports profile + binding flow', async ({ page }) => {
    await mockAPIs(page);

    const hosts = [{ id: 'host-1', name: 'prod-host-1' }, { id: 'host-2', name: 'staging-host-2' }];
    const profiles: Array<Record<string, unknown>> = [];
    const bindings: Array<Record<string, unknown>> = [];
    let bindingSaveCalls = 0;
    let lastBindingBody: Record<string, unknown> | null = null;
    let bindingDeleteCalls = 0;
    let governanceResolveCalls = 0;
    let profilePatchCalls = 0;
    let lastProfilePatchBody: Record<string, unknown> | null = null;
    let profileTestCalls = 0;
    let lastProfileTestBody: Record<string, unknown> | null = null;

    await page.route('**/api/v1/remote/hosts', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ result: 'ok', hosts }),
      }),
    );

    await page.route('**/api/v1/provider-profiles', async (route) => {
      const req = route.request();
      const path = new URL(req.url()).pathname;
      if (path !== '/api/v1/provider-profiles') {
        return route.fallback();
      }
      if (req.method() === 'GET') {
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ result: 'ok', profiles }),
        });
      }
      if (req.method() === 'POST') {
        const body = req.postDataJSON() as Record<string, unknown>;
        const profile = { id: `profile-${profiles.length + 1}`, ...body };
        profiles.push(profile);
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ result: 'ok', profile }),
        });
      }
      return route.fallback();
    });

    await page.route('**/api/v1/provider-profiles/*/test', async (route) => {
      if (route.request().method() !== 'POST') return route.fallback();
      profileTestCalls += 1;
      lastProfileTestBody = route.request().postDataJSON() as Record<string, unknown>;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ result: 'ok', test: { ok: true } }),
      });
    });

    await page.route('**/api/v1/provider-profiles/*', async (route) => {
      const req = route.request();
      const path = new URL(req.url()).pathname;
      if (req.method() === 'PATCH') {
        profilePatchCalls += 1;
        lastProfilePatchBody = req.postDataJSON() as Record<string, unknown>;
        const profileID = path.split('/').slice(-1)[0];
        const idx = profiles.findIndex((p) => String(p.id) === profileID);
        if (idx >= 0) {
          profiles[idx] = {
            ...profiles[idx],
            ...lastProfilePatchBody,
            id: profileID,
          };
        }
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ result: 'ok', profile: idx >= 0 ? profiles[idx] : null }),
        });
      }
      if (req.method() !== 'DELETE') return route.fallback();
      const profileID = path.split('/').slice(-1)[0];
      const idx = profiles.findIndex((p) => String(p.id) === profileID);
      if (idx >= 0) profiles.splice(idx, 1);
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ result: 'ok', deleted: true }),
      });
    });

    await page.route('**/api/v1/provider-bindings', async (route) => {
      const req = route.request();
      if (req.method() === 'GET') {
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ result: 'ok', bindings }),
        });
      }
      if (req.method() === 'POST') {
        const body = req.postDataJSON() as Record<string, unknown>;
        bindingSaveCalls += 1;
        lastBindingBody = body;
        const binding = { id: `binding-${bindings.length + 1}`, ...body };
        bindings.push(binding);
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ result: 'ok', binding }),
        });
      }
      return route.fallback();
    });
    await page.route('**/api/v1/provider-bindings/*', async (route) => {
      const req = route.request();
      if (req.method() !== 'DELETE') return route.fallback();
      bindingDeleteCalls += 1;
      const path = new URL(req.url()).pathname;
      const bindingID = decodeURIComponent(path.split('/').slice(-1)[0] || '');
      const idx = bindings.findIndex((b) => String(b.id) === bindingID);
      if (idx >= 0) bindings.splice(idx, 1);
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ result: 'ok', deleted: true }),
      });
    });
    await page.route('**/api/v1/provider-governance/resolve*', async (route) => {
      governanceResolveCalls += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          result: 'ok',
          resolution: {
            source: 'host',
            hostId: 'host-1',
            agentId: 'zeroclaw',
            bindingId: 'binding-host-1',
            bindingTargetType: 'host',
            bindingTargetId: 'host-1',
            profileId: 'profile-1',
            profileName: 'openai-gpt5',
            provider: 'openai',
            model: 'gpt-5-mini',
            syncMode: 'always_push',
          },
        }),
      });
    });

    await loginWithToken(page, '/#/profiles');

    await page.fill('#profile-name', 'openai-gpt5');
    await page.fill('#profile-provider', 'openai');
    await page.fill('#profile-model', 'gpt-5');
    await page.fill('#profile-base-url', 'https://api.openai.com/v1');
    await page.fill('#profile-auth-ref', 'env:OPENAI_API_KEY');
    await page.selectOption('#profile-enabled', 'true');
    await page.click('#profile-save');

    await expect(page.locator('#profiles-list .agent-card h4', { hasText: 'openai-gpt5' })).toBeVisible();
    await page.locator('#profiles-list .agent-card').first().locator('button', { hasText: 'Edit' }).click();
    await expect(page.locator('#profile-editor-state')).toContainText('Editing profile');
    await expect(page.locator('#profile-save')).toContainText('Update Profile');
    await page.fill('#profile-model', 'gpt-5-mini');
    await page.click('#profile-save');
    await expect.poll(() => profilePatchCalls).toBe(1);
    expect(lastProfilePatchBody?.model).toBe('gpt-5-mini');
    await expect(page.locator('#profiles-list .agent-card').first().locator('.instance-meta')).toContainText('openai/gpt-5-mini');

    await page.selectOption('#binding-target-type', 'host');
    await page.fill('#binding-target-id', 'host-1');
    await page.click('#binding-save');

    await expect(page.locator('#bindings-list .agent-card h4', { hasText: 'host: host-1' })).toBeVisible();
    await expect.poll(() => bindingSaveCalls).toBe(1);
    expect(lastBindingBody?.profileId).toBe('profile-1');
    expect(lastBindingBody?.targetType).toBe('host');
    expect(lastBindingBody?.targetId).toBe('host-1');

    page.once('dialog', (dialog) => dialog.accept());
    await page.locator('#bindings-list .agent-card').first().locator('button', { hasText: 'Delete' }).click();
    await expect.poll(() => bindingDeleteCalls).toBe(1);
    await expect(page.locator('#bindings-list')).toContainText('No provider bindings configured.');

    await page.selectOption('#profile-test-host', 'host-2');
    await page.locator('#profiles-list .agent-card').first().locator('button', { hasText: 'Test' }).click();
    await expect.poll(() => profileTestCalls).toBe(1);
    expect(lastProfileTestBody?.hostId).toBe('host-2');
    await expect(page.locator('#profiles-msg')).toContainText('Profile test succeeded');

    await page.selectOption('#governance-preview-host', 'host-1');
    await page.fill('#governance-preview-agent', 'zeroclaw');
    await page.click('#governance-preview-resolve');
    await expect.poll(() => governanceResolveCalls).toBe(1);
    await expect(page.locator('#governance-preview-out')).toContainText('openai/gpt-5-mini');
    await expect(page.locator('#governance-preview-out')).toContainText('source=host');
  });

  test('remote chat page supports remote and local target streaming', async ({ page }) => {
    await mockAPIs(page);

    let lastChatBody: Record<string, unknown> | null = null;

    await page.route('**/api/v1/remote/hosts', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          result: 'ok',
          hosts: [{ id: 'host-1', name: 'prod-host-1' }],
        }),
      }),
    );

    await page.route('**/api/v1/remote/hosts/*/instances', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          result: 'ok',
          instances: [{ id: 'host-1:main', agentId: 'main', runtimeState: 'running' }],
        }),
      }),
    );

    await page.route('**/api/v1/provider-profiles', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          result: 'ok',
          profiles: [{ id: 'profile-1', name: 'openai-gpt5', provider: 'openai', model: 'gpt-5' }],
        }),
      }),
    );

    await page.route('**/api/v1/provider-bindings', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ result: 'ok', binding: { id: 'binding-1' } }),
      }),
    );

    await page.route('**/api/v1/chat/stream', async (route) => {
      lastChatBody = route.request().postDataJSON() as Record<string, unknown>;
      const target = String(lastChatBody.target || '');
      const reply = target === 'local' ? 'local-instance-reply' : 'remote-instance-reply';
      return route.fulfill({
        status: 200,
        contentType: 'text/event-stream',
        body: [
          'data: {"type":"start","requestId":"req-chat-1"}',
          '',
          `data: {"type":"text-delta","delta":"${reply}"}`,
          '',
          'data: {"type":"session","sessionId":"sess-chat-1"}',
          '',
          'data: {"type":"finish","finishReason":"stop"}',
          '',
        ].join('\n'),
      });
    });

    await loginWithToken(page, '/#/remote-chat');

    await page.fill('#remote-chat-input', 'hello remote');
    await page.click('#remote-chat-send');
    await expect(page.locator('#remote-chat-messages')).toContainText('remote-instance-reply');
    expect(lastChatBody).toBeTruthy();
    expect(lastChatBody?.target).toBe('remote');
    expect(lastChatBody?.hostId).toBe('host-1');
    expect(lastChatBody?.agentId).toBe('main');

    await page.selectOption('#remote-chat-target', 'local');
    await page.waitForTimeout(50);

    await page.fill('#remote-chat-input', 'hello local');
    await page.click('#remote-chat-send');
    await expect(page.locator('#remote-chat-messages')).toContainText('local-instance-reply');
    expect(lastChatBody).toBeTruthy();
    expect(lastChatBody?.target).toBe('local');
  });
});
