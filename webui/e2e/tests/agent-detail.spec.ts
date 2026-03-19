import { expect, test } from '@playwright/test';
import { loginWithToken, mockAPIs } from './helpers';

test.describe('Agent Detail', () => {
  test('renders agent status and supports start/stop actions', async ({ page }) => {
    await mockAPIs(page);
    let startCalls = 0;
    let stopCalls = 0;
    let skillToggleCalls = 0;
    let skillEnabled = true;
    let skillSearchCalls = 0;
    let skillInstallCalls = 0;
    let skillUpdateCalls = 0;
    let skillUninstallCalls = 0;
    let mcpToggleCalls = 0;
    let mcpEnabled = true;
    let mcpAttached = true;
    let mcpConfig = '{"mode":"read"}';
    let cronRunCalls = 0;
    let cronPauseCalls = 0;
    let cronResumeCalls = 0;
    let cronPaused = false;
    let syncedModels = false;
    let defaultProfile = 'openrouter-fast';
    let setDefaultCalls = 0;
    let updatedProfile = {
      profileName: 'openrouter-safe',
      modelAlias: 'flash-safe',
      modelId: 'deepseek/deepseek-chat-v3-0324',
      providerId: 'openrouter',
      protocolFamily: 'openai-compatible',
      baseUrl: '',
      authMethod: '',
      timeoutMs: 0,
      retryBudget: 0,
      fallbackStrategy: '',
      fallbackGroup: 'openrouter:flash',
      aliasGroupSize: 2,
      primary: false,
    };

    await page.route('**/api/v1/agents/agent-alpha/capabilities', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          skillSummary: { installedCount: 1, enabledCount: skillEnabled ? 1 : 0, disabledCount: skillEnabled ? 0 : 1 },
          skills: [{ name: 'toolbox', enabled: skillEnabled, source: 'catalog', version: 'builtin', targetVersion: 'v2.0.0', health: 'degraded', updateStatus: 'update_available', updateAvailable: true }],
          mcp: {
            servers: [{ name: 'repo', health: mcpAttached ? (mcpEnabled ? 'healthy' : 'stopped') : 'detached', enabled: mcpEnabled, attached: mcpAttached, manageable: true, visibleToolCount: 1, hiddenToolCount: 0 }],
            visibleTools: [{ name: 'repo_search', description: 'Search code' }],
          },
        }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/launcher', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          agentId: 'agent-alpha',
          status: { id: 'agent-alpha', runtimeState: 'running', health: 'healthy' },
          heartbeat: { state: 'fresh', ageSeconds: 12, lastActivityAt: '2026-03-12T03:59:48Z' },
          memory: { contractId: 'memory-alpha', contractDigest: 'sha256:abc' },
          providerReadiness: { provider: 'openrouter', authMode: 'api_key', credentialConfigured: true, ready: true },
          lastModelRun: {
            requestedAlias: 'flash-safe',
            requestedModel: 'deepseek/deepseek-chat-v3-0324',
            resolvedModel: 'deepseek/deepseek-chat-v3-0324',
            resolvedProfile: 'openrouter-safe',
            fallbackGroup: 'openrouter:flash',
            selectionStrategy: 'explicit_model',
            selectionOrdinal: 1,
            overrideHit: true,
            fallbackHit: true,
            lastRunAt: '2026-03-12T00:05:00Z',
          },
          modelSurface: {
            defaultProfile,
            profiles: [
              { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: true },
              { profileName: 'openrouter-safe', modelAlias: 'flash-safe', modelId: 'deepseek/deepseek-chat-v3-0324', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: false },
            ],
          },
          cron: {
            count: 2,
            nextRunAt: '2026-03-13T00:00:00Z',
            lastRunAt: '2026-03-12T23:55:00Z',
            lastResult: 'succeeded',
            jobs: [
              { id: 'cron-1', prompt: 'check launcher', nextRunAt: '2026-03-13T00:00:00Z', lastRunAt: '2026-03-12T23:55:00Z', lastResult: cronPaused ? 'paused' : 'succeeded', paused: cronPaused, history: [{ trigger: 'manual', result: 'succeeded' }] },
              { id: 'cron-2', prompt: 'refresh heartbeat', nextRunAt: '2026-03-13T01:00:00Z', lastResult: 'scheduled' },
            ],
          },
          delegation: {
            count: 1,
            jobs: [{ jobId: 'subagent-1', task: 'collect diagnostics', status: 'completed', summary: 'summary-ready', result: 'done', updatedAt: '2026-03-12T00:06:00Z' }],
          },
          sessions: {
            count: 1,
            sessions: [{ key: 'telegram:prod', messageCount: 8, summaryLength: 64, updatedAt: '2026-03-12T00:07:00Z' }],
          },
          session: { instanceId: 'instance-1', channel: 'telegram', isolation: true, runtimeState: 'running' },
          capabilities: {
            skillSummary: { installedCount: 1, enabledCount: skillEnabled ? 1 : 0, disabledCount: skillEnabled ? 0 : 1 },
            skills: [{ name: 'toolbox', enabled: skillEnabled, source: 'catalog', version: 'builtin', targetVersion: 'v2.0.0', health: 'degraded', updateStatus: 'update_available', updateAvailable: true }],
            mcp: {
              servers: [{ name: 'repo', health: mcpAttached ? (mcpEnabled ? 'healthy' : 'stopped') : 'detached', enabled: mcpEnabled, attached: mcpAttached, manageable: true, visibleToolCount: 1, hiddenToolCount: 0 }],
              visibleTools: [{ name: 'repo_search', description: 'Search code' }],
            },
          },
        }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/subagents/subagent-1', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          jobId: 'subagent-1',
          task: 'collect diagnostics',
          status: 'completed',
          summary: 'summary-ready',
          result: 'done',
          updatedAt: '2026-03-12T00:06:00Z',
        }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/models', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          agentId: 'agent-alpha',
          instanceId: 'instance-1',
          configPath: '/tmp/agent-alpha/config.toml',
          modelSurface: {
            defaultProfile: syncedModels ? 'openrouter-safe' : defaultProfile,
            profiles: syncedModels || defaultProfile === 'openrouter-safe'
              ? [
                  { ...updatedProfile, primary: true },
                  { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: false },
                ]
              : [
                  { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: true },
                  updatedProfile,
                ],
          },
        }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/models/sync', async (route) => {
      syncedModels = true;
      defaultProfile = 'openrouter-safe';
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          agentId: 'agent-alpha',
          instanceId: 'instance-1',
          configPath: '/tmp/agent-alpha/config.toml',
          synced: true,
          modelSurface: {
            defaultProfile: 'openrouter-safe',
            profiles: [
              { profileName: 'openrouter-safe', modelAlias: 'flash-safe', modelId: 'deepseek/deepseek-chat-v3-0324', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: true },
              { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: false },
            ],
          },
        }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/models/discover', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          agentId: 'agent-alpha',
          instanceId: 'instance-1',
          configPath: '/tmp/agent-alpha/config.toml',
          driftState: 'drifted',
          driftReason: 'stored model surface differs from config-discovered model surface',
          modelSurface: {
            defaultProfile,
            profiles: [
              { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: true },
              updatedProfile,
            ],
          },
          discoveredModelSurface: {
            defaultProfile: 'openrouter-safe',
            profiles: [
              { profileName: 'openrouter-safe', modelAlias: 'flash-safe', modelId: 'deepseek/deepseek-chat-v3-0324', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: true },
              { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: false },
            ],
          },
        }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/models/default', async (route) => {
      setDefaultCalls += 1;
      defaultProfile = 'openrouter-safe';
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          agentId: 'agent-alpha',
          instanceId: 'instance-1',
          configPath: '/tmp/agent-alpha/config.toml',
          modelSurface: {
            defaultProfile: 'openrouter-safe',
            profiles: [
              { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: true },
              { profileName: 'openrouter-safe', modelAlias: 'flash-safe', modelId: 'deepseek/deepseek-chat-v3-0324', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: false },
            ],
          },
        }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/models/profile', async (route) => {
      const body = route.request().postDataJSON() as Record<string, unknown>;
      updatedProfile = {
        profileName: 'openrouter-safe',
        modelAlias: String(body.modelAlias || ''),
        modelId: String(body.modelId || ''),
        providerId: String(body.providerId || ''),
        protocolFamily: String(body.providerId || '') === 'anthropic' ? 'anthropic' : 'openai-compatible',
        baseUrl: String(body.baseUrl || ''),
        authMethod: String(body.authMethod || ''),
        timeoutMs: Number(body.timeoutMs || 0),
        retryBudget: Number(body.retryBudget || 0),
        fallbackStrategy: String(body.fallbackStrategy || ''),
        fallbackGroup: 'anthropic:flash-safe-v2',
        aliasGroupSize: 1,
        primary: false,
      };
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          agentId: 'agent-alpha',
          instanceId: 'instance-1',
          configPath: '/tmp/agent-alpha/config.toml',
          modelSurface: {
            defaultProfile,
            profiles: [
              { profileName: 'openrouter-fast', modelAlias: 'flash', modelId: 'google/gemini-2.0-flash-001', providerId: 'openrouter', protocolFamily: 'openai-compatible', fallbackGroup: 'openrouter:flash', aliasGroupSize: 2, primary: true },
              updatedProfile,
            ],
          },
        }),
      });
    });

    await page.route('**/api/v1/agents/agent-alpha/start', async (route) => {
      startCalls += 1;
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"ok":true}' });
    });
    await page.route('**/api/v1/agents/agent-alpha/stop', async (route) => {
      stopCalls += 1;
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"ok":true}' });
    });
    await page.route('**/api/v1/agents/agent-alpha/skills/toolbox', async (route) => {
      skillToggleCalls += 1;
      skillEnabled = false;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          skillSummary: { installedCount: 1, enabledCount: 0, disabledCount: 1 },
          skills: [{ name: 'toolbox', enabled: false, source: 'catalog', version: 'builtin', health: 'degraded', updateStatus: 'update_available', updateAvailable: true }],
          mcp: {
            servers: [{ name: 'repo', health: mcpAttached ? 'healthy' : 'detached', enabled: mcpEnabled, attached: mcpAttached, visibleToolCount: 1, hiddenToolCount: 0 }],
            visibleTools: [{ name: 'repo_search', description: 'Search code' }],
          },
        }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/skills/search**', async (route) => {
      skillSearchCalls += 1;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          skills: [{ name: 'workspace-inspection', summary: 'Inspect workspace state.', source: 'catalog', version: 'v1.2.3', health: 'healthy', updateStatus: 'current', updateAvailable: false }],
        }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/skills/install', async (route) => {
      skillInstallCalls += 1;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ name: 'workspace-inspection', summary: 'Inspect workspace state.', source: 'catalog', version: 'v1.2.3', health: 'healthy', updateStatus: 'current', updateAvailable: false }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/skills/update', async (route) => {
      skillUpdateCalls += 1;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ name: 'toolbox', summary: 'Core toolbox', source: 'catalog', version: 'builtin', targetVersion: 'v2.0.0', health: 'degraded', updateStatus: 'update_available', updateAvailable: true }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/skills/uninstall', async (route) => {
      skillUninstallCalls += 1;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ name: 'toolbox', summary: 'Core toolbox', source: 'catalog', version: 'builtin', health: 'degraded', updateStatus: 'update_available', updateAvailable: true }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/mcp/repo/attach', async (route) => {
      mcpAttached = true;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          name: 'repo',
          health: 'healthy',
          enabled: mcpEnabled,
          attached: true,
          manageable: true,
          visibleToolCount: 1,
          hiddenToolCount: 1,
          healthDetail: 'connected to repository index',
          remediationHint: 'Disable MCP if repository indexing becomes noisy.',
          configDigest: 'sha256:test-config',
          configSummary: mcpConfig,
          visibleTools: [{ name: 'repo_search', description: 'Search code' }],
          hiddenTools: [{ name: 'repo_admin', description: 'Admin index' }],
        }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/mcp/repo/detach', async (route) => {
      mcpAttached = false;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          name: 'repo',
          health: 'detached',
          enabled: mcpEnabled,
          attached: false,
          manageable: true,
          visibleToolCount: 1,
          hiddenToolCount: 1,
          healthDetail: 'detached from runtime',
          remediationHint: 'Attach MCP before expecting tools to appear.',
          configDigest: 'sha256:test-config',
          configSummary: mcpConfig,
          visibleTools: [{ name: 'repo_search', description: 'Search code' }],
          hiddenTools: [{ name: 'repo_admin', description: 'Admin index' }],
        }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/mcp/repo/config', async (route) => {
      const body = route.request().postDataJSON() as Record<string, unknown>;
      mcpConfig = String(body.config || '');
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          name: 'repo',
          health: mcpAttached ? 'healthy' : 'detached',
          enabled: mcpEnabled,
          attached: mcpAttached,
          manageable: true,
          visibleToolCount: 1,
          hiddenToolCount: 1,
          healthDetail: mcpAttached ? 'connected to repository index' : 'detached from runtime',
          remediationHint: mcpAttached ? 'Disable MCP if repository indexing becomes noisy.' : 'Attach MCP before expecting tools to appear.',
          configDigest: 'sha256:test-config',
          configSummary: mcpConfig,
          visibleTools: [{ name: 'repo_search', description: 'Search code' }],
          hiddenTools: [{ name: 'repo_admin', description: 'Admin index' }],
        }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/mcp/repo', async (route) => {
      if (route.request().method() === 'POST') {
        mcpToggleCalls += 1;
        mcpEnabled = false;
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            mcp: {
              servers: [{ name: 'repo', health: mcpAttached ? 'stopped' : 'detached', enabled: false, attached: mcpAttached, manageable: true, visibleToolCount: 1, hiddenToolCount: 0 }],
              visibleTools: [],
            },
          }),
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          name: 'repo',
          health: mcpAttached ? 'healthy' : 'detached',
          enabled: mcpEnabled,
          attached: mcpAttached,
          manageable: true,
          visibleToolCount: 1,
          hiddenToolCount: 1,
          healthDetail: mcpAttached ? 'connected to repository index' : 'detached from runtime',
          remediationHint: mcpAttached ? 'Disable MCP if repository indexing becomes noisy.' : 'Attach MCP before expecting tools to appear.',
          configDigest: 'sha256:test-config',
          configSummary: mcpConfig,
          visibleTools: [{ name: 'repo_search', description: 'Search code' }],
          hiddenTools: [{ name: 'repo_admin', description: 'Admin index' }],
        }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/cron/cron-1/run', async (route) => {
      cronRunCalls += 1;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ id: 'cron-1', prompt: 'check launcher', lastResult: 'succeeded', paused: cronPaused, history: [{ trigger: 'manual', result: 'succeeded' }] }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/cron/cron-1/pause', async (route) => {
      cronPauseCalls += 1;
      cronPaused = true;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ id: 'cron-1', prompt: 'check launcher', lastResult: 'paused', paused: true, history: [{ trigger: 'manual', result: 'succeeded' }] }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/cron/cron-1/resume', async (route) => {
      cronResumeCalls += 1;
      cronPaused = false;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ id: 'cron-1', prompt: 'check launcher', lastResult: 'resumed', paused: false, history: [{ trigger: 'manual', result: 'succeeded' }] }),
      });
    });

    await loginWithToken(page, '/#/agents/agent-alpha');

    await expect(page.locator('#view-agent-detail')).toBeVisible();
    await expect(page.locator('#agent-detail-content')).toContainText('Agent: agent-alpha');
    await expect(page.locator('#agent-detail-content')).toContainText('Heartbeat');
    await expect(page.locator('#agent-detail-content')).toContainText('fresh');
    await expect(page.locator('#agent-detail-content')).toContainText('openrouter');
    await expect(page.locator('#agent-detail-content')).toContainText('memory-alpha');
    await expect(page.locator('#agent-detail-content')).toContainText('Cron');
    await expect(page.locator('#agent-detail-content')).toContainText('2 job(s)');
    await expect(page.locator('#agent-detail-content')).toContainText('check launcher');
    await expect(page.locator('#agent-detail-content')).toContainText('manual');
    await expect(page.locator('#agent-detail-content')).toContainText('1 installed · 1 enabled · 0 disabled');
    await expect(page.locator('#agent-detail-content')).toContainText('target=v2.0.0');
    await expect(page.locator('#agent-detail-content')).toContainText('health=degraded');
    await expect(page.locator('#agent-detail-content')).toContainText('update_available');
    await expect(page.locator('#agent-detail-content')).toContainText('update available');
    await expect(page.locator('#agent-detail-content')).toContainText('"runtimeState": "running"');
    await expect(page.locator('#agent-detail-content')).toContainText('/tmp/agent-alpha/config.toml');
    await expect(page.locator('#agent-detail-content')).toContainText('Model Runtime Trace');
    await expect(page.locator('#agent-detail-content')).toContainText('requested=flash-safe');
    await expect(page.locator('#agent-detail-content')).toContainText('resolved=deepseek/deepseek-chat-v3-0324');
    await expect(page.locator('#agent-detail-content')).toContainText('override hit');
    await expect(page.locator('#agent-detail-content')).toContainText('fallback hit');
    await expect(page.locator('#agent-detail-content')).toContainText('last=2026-03-12T00:05:00Z');
    await expect(page.locator('#agent-detail-content')).toContainText('Recent Delegation Jobs');
    await expect(page.locator('#agent-detail-content')).toContainText('subagent-1');
    await page.getByRole('button', { name: 'Inspect subagent-1 delegation' }).click();
    await expect(page.locator('#agent-detail-content')).toContainText('subagent-1 Delegation Detail');
    await expect(page.locator('#agent-detail-content')).toContainText('summary-ready');
    await expect(page.locator('#agent-detail-content')).toContainText('Recent Sessions');
    await expect(page.locator('#agent-detail-content')).toContainText('telegram:prod');

    await page.getByRole('button', { name: 'Disable', exact: true }).click();
    await expect.poll(() => skillToggleCalls).toBe(1);
    await expect(page.locator('#agent-detail-content')).toContainText('Skill toolbox disabled.');

    await page.getByPlaceholder('Search skills').fill('workspace');
    await page.getByRole('button', { name: 'Search Skills' }).click();
    await expect.poll(() => skillSearchCalls).toBe(1);
    await expect(page.locator('#agent-detail-content')).toContainText('workspace-inspection');

    await page.getByRole('button', { name: 'Install workspace-inspection' }).click();
    await expect.poll(() => skillInstallCalls).toBe(1);
    await expect(page.locator('#agent-detail-content')).toContainText('Installed skill workspace-inspection.');

    await page.getByPlaceholder('Pin version for toolbox').fill('v2.0.0');
    await page.getByRole('button', { name: 'Update toolbox' }).click();
    await expect.poll(() => skillUpdateCalls).toBe(1);
    await expect(page.locator('#agent-detail-content')).toContainText('Updated skill toolbox.');

    await page.getByRole('button', { name: 'Disable MCP' }).click();
    await expect.poll(() => mcpToggleCalls).toBe(1);
    await expect(page.locator('#agent-detail-content')).toContainText('MCP server repo disabled.');

    await page.getByRole('button', { name: 'Inspect repo MCP' }).click();
    await expect(page.locator('#agent-detail-content')).toContainText('connected to repository index');
    await expect(page.locator('#agent-detail-content')).toContainText('repo_admin');
    await expect(page.locator('#agent-detail-content')).toContainText('sha256:test-config');
    await expect(page.getByLabel('repo MCP config')).toHaveValue('{"mode":"read"}');

    await page.getByRole('button', { name: 'Detach repo', exact: true }).click();
    await expect(page.locator('#agent-detail-content')).toContainText('MCP server repo detached.');

    await page.getByRole('button', { name: 'Attach repo', exact: true }).click();
    await expect(page.locator('#agent-detail-content')).toContainText('MCP server repo attached.');

    await page.getByLabel('repo MCP config').fill('{"mode":"write"}');
    await page.getByRole('button', { name: 'Save repo config' }).click();
    await expect(page.locator('#agent-detail-content')).toContainText('MCP config for repo updated.');

    await page.getByRole('button', { name: 'Run cron-1 now' }).click();
    await expect.poll(() => cronRunCalls).toBe(1);
    await expect(page.locator('#agent-detail-content')).toContainText('Cron job cron-1 run requested.');

    await page.getByRole('button', { name: 'Pause cron-1' }).click();
    await expect.poll(() => cronPauseCalls).toBe(1);
    await expect(page.locator('#agent-detail-content')).toContainText('Cron job cron-1 paused.');
    await expect(page.locator('#agent-detail-content')).toContainText(
      'One or more cron jobs are paused. Resume or cancel them to restore scheduled automation.',
    );

    await page.getByRole('button', { name: 'Resume cron-1' }).click();
    await expect.poll(() => cronResumeCalls).toBe(1);
    await expect(page.locator('#agent-detail-content')).toContainText('Cron job cron-1 resumed.');

    await page.getByRole('button', { name: 'Set default openrouter-safe' }).click();
    await expect.poll(() => setDefaultCalls).toBe(1);
    await expect(page.locator('#agent-detail-content')).toContainText('Default model profile set to openrouter-safe.');
    await expect(page.locator('#agent-detail-content')).toContainText('default=openrouter-safe');

    await page.getByRole('button', { name: /Sync models/i }).click();
    await expect(page.locator('#agent-detail-content')).toContainText('Model surface synced.');
    await expect(page.locator('#agent-detail-content')).toContainText('openrouter-safe');

    await page.getByRole('button', { name: 'Inspect model drift' }).click();
    await expect(page.locator('#agent-detail-content')).toContainText('Model discovery drifted.');
    await expect(page.locator('#agent-detail-content')).toContainText('discovered-default=flash-safe -> deepseek/deepseek-chat-v3-0324');

    await page.getByRole('button', { name: 'Edit profile openrouter-safe' }).click();
    await page.getByLabel('Model alias for openrouter-safe').fill('flash-safe-v2');
    await page.getByLabel('Model ID for openrouter-safe').fill('anthropic/claude-sonnet-4.6');
    await page.getByLabel('Provider for openrouter-safe').fill('anthropic');
    await page.getByLabel('Base URL for openrouter-safe').fill('https://api.anthropic.com/v1');
    await page.getByLabel('Auth method for openrouter-safe').fill('api_key');
    await page.getByLabel('Timeout ms for openrouter-safe').fill('60000');
    await page.getByLabel('Retry budget for openrouter-safe').fill('4');
    await page.getByLabel('Fallback strategy for openrouter-safe').fill('round_robin');
    await page.getByRole('button', { name: 'Save profile openrouter-safe' }).click();
    await expect(page.locator('#agent-detail-content')).toContainText('Model profile openrouter-safe updated.');
    await expect(page.locator('#agent-detail-content')).toContainText('flash-safe-v2');
    await expect(page.locator('#agent-detail-content')).toContainText('anthropic/claude-sonnet-4.6');

    await page.getByRole('button', { name: 'Start' }).evaluate((element: HTMLButtonElement) => element.click());
    await expect.poll(() => startCalls).toBe(1);
    await expect(page.locator('#agent-detail-content')).toContainText('Agent start requested.');

    const stopButton = page.getByRole('button', { name: 'Stop' });
    await expect(stopButton).toBeEnabled();
    await stopButton.evaluate((element: HTMLButtonElement) => element.click());
    await expect.poll(() => stopCalls).toBe(1);
    await expect(page.locator('#agent-detail-content')).toContainText('Agent stop requested.');

    await page.getByRole('button', { name: 'Back' }).evaluate((element: HTMLButtonElement) => element.click());
    await expect(page).toHaveURL(/\/dashboard$/);
    await expect(page.locator('.agent-card').first()).toBeVisible();
  });
});
