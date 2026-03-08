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

export const MOCK_EXECUTIONS = [
  {
    id: 'exec-running',
    goal: 'Investigate checkout latency',
    status: 'running',
    updatedAt: '2026-03-08T11:05:00Z',
    taskUnits: [
      { id: 'task-1', input: 'collect traces', hostId: 'local', agentId: 'zeroclaw' },
      { id: 'task-2', input: 'summarize traces', hostId: 'host-1', agentId: 'picoclaw' },
    ],
    results: [
      { taskId: 'task-1', status: 'completed', hostId: 'local', agentId: 'zeroclaw', output: 'trace bundle ready', attempts: 1, latencyMs: 42 },
    ],
  },
  {
    id: 'exec-complete',
    goal: 'Prepare release notes',
    status: 'completed',
    updatedAt: '2026-03-08T10:30:00Z',
    taskUnits: [
      { id: 'task-1', input: 'collect merged PRs', hostId: 'local', agentId: 'zeroclaw' },
    ],
    results: [
      { taskId: 'task-1', status: 'completed', hostId: 'local', agentId: 'zeroclaw', output: 'release notes draft ready', attempts: 1, latencyMs: 18 },
    ],
  },
];

function cloneJSON<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

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
          builtin: [
            { id: 'openai', name: 'OpenAI', auth_mode: 'api_key', env_var: 'OPENAI_API_KEY', example_model: 'openai/gpt-4o', description: 'OpenAI API' },
          ],
          'API Key': [
            { id: 'openai', name: 'OpenAI', auth_mode: 'api_key', env_var: 'OPENAI_API_KEY', example_model: 'openai/gpt-4o', description: 'OpenAI API' },
          ],
        },
      }),
    }),
  );

  await page.route('**/api/v1/remote/hosts', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        hosts: [{ id: 'host-1', name: 'prod-host-1' }],
      }),
    }),
  );

  await page.route('**/api/v1/orchestrator/plans', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        plan: {
          goal: 'Mock plan',
          approvalScope: 'infrastructure_only',
          maxConcurrency: 1,
          plannerTasks: [{ id: 'task-1', input: 'noop', agentId: 'zeroclaw' }],
          requiredWorkers: [{ hostId: 'local', agentId: 'zeroclaw', count: 1 }],
          taskUnits: [{ id: 'task-1', input: 'noop', hostId: 'local', agentId: 'zeroclaw', timeoutMs: 60000, retryBudget: 0 }],
        },
      }),
    }),
  );

  await page.route('**/api/v1/orchestrator/executions', (route) => {
    if (route.request().method() === 'GET') {
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ result: 'ok', executions: [] }),
      });
    }
    return route.fulfill({
      status: 201,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        execution: {
          id: 'exec-created',
          goal: 'Mock execution',
          status: 'pending_authorization',
          taskUnits: [{ id: 'task-1', input: 'noop', hostId: 'local', agentId: 'zeroclaw' }],
          results: [],
        },
      }),
    });
  });

  await page.route('**/api/v1/orchestrator/executions/*/authorize', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        execution: {
          id: 'exec-created',
          goal: 'Mock execution',
          status: 'running',
          taskUnits: [{ id: 'task-1', input: 'noop', hostId: 'local', agentId: 'zeroclaw' }],
          results: [],
        },
      }),
    }),
  );

  await page.route('**/api/v1/orchestrator/executions/*/cancel', (route) =>
    route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        execution: {
          id: 'exec-created',
          goal: 'Mock execution',
          status: 'cancelled',
          taskUnits: [{ id: 'task-1', input: 'noop', hostId: 'local', agentId: 'zeroclaw' }],
          results: [{ taskId: 'task-1', status: 'failed', error: 'cancelled', hostId: 'local', agentId: 'zeroclaw' }],
        },
      }),
    }),
  );

  await page.route('**/api/v1/orchestrator/executions/*', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        execution: {
          id: 'exec-created',
          goal: 'Mock execution',
          status: 'running',
          taskUnits: [{ id: 'task-1', input: 'noop', hostId: 'local', agentId: 'zeroclaw' }],
          results: [],
        },
        workers: [{ id: 'worker-1', hostId: 'local', agentId: 'zeroclaw', state: 'leased' }],
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

export async function mockOrchestrationAPIs(page: Page) {
  const executions = cloneJSON(MOCK_EXECUTIONS);
  let nextExecutionID = 1;

  await page.route('**/api/v1/orchestrator/plans', async (route) => {
    const body = route.request().postDataJSON() as Record<string, unknown>;
    const goal = String(body.goal || '').trim();
    const provider = String(body.provider || '').trim();
    const hostIds = Array.isArray(body.hostIds) ? (body.hostIds as string[]) : [];
    const maxConcurrency = Number(body.maxConcurrency || 0) || 2;
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        plan: {
          goal,
          provider,
          hostIds,
          approvalScope: 'infrastructure_only',
          maxConcurrency,
          plannerTasks: [
            { id: 'task-1', input: 'collect context', agentId: 'zeroclaw' },
            { id: 'task-2', input: 'draft summary', agentId: 'picoclaw' },
          ],
          requiredWorkers: [
            { hostId: hostIds[0] || 'local', agentId: 'zeroclaw', count: 1 },
            { hostId: hostIds[1] || hostIds[0] || 'local', agentId: 'picoclaw', count: 1 },
          ],
          taskUnits: [
            { id: 'task-1', input: 'collect context', hostId: hostIds[0] || 'local', agentId: 'zeroclaw', timeoutMs: 60000, retryBudget: 0 },
            { id: 'task-2', input: 'draft summary', hostId: hostIds[1] || hostIds[0] || 'local', agentId: 'picoclaw', timeoutMs: 60000, retryBudget: 0 },
          ],
        },
      }),
    });
  });

  await page.route('**/api/v1/orchestrator/executions', async (route) => {
    if (route.request().method() === 'GET') {
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ result: 'ok', executions }),
      });
    }

    const body = route.request().postDataJSON() as Record<string, unknown>;
    const execution = {
      id: `exec-preview-${nextExecutionID++}`,
      goal: String(body.goal || '').trim(),
      status: 'pending_authorization',
      updatedAt: '2026-03-08T12:00:00Z',
      taskUnits: cloneJSON((body.taskUnits as Array<Record<string, unknown>>) || []),
      results: [],
    };
    executions.unshift(execution);
    return route.fulfill({
      status: 201,
      contentType: 'application/json',
      body: JSON.stringify({ result: 'ok', execution }),
    });
  });

  await page.route('**/api/v1/orchestrator/executions/*/authorize', async (route) => {
    const path = new URL(route.request().url()).pathname;
    const executionID = path.split('/')[5];
    const execution = executions.find((item) => item.id === executionID);
    if (!execution) {
      return route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ result: 'error', message: 'not found' }) });
    }
    execution.status = 'running';
    execution.updatedAt = '2026-03-08T12:01:00Z';
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ result: 'ok', execution }),
    });
  });

  await page.route('**/api/v1/orchestrator/executions/*/cancel', async (route) => {
    const path = new URL(route.request().url()).pathname;
    const executionID = path.split('/')[5];
    const execution = executions.find((item) => item.id === executionID);
    if (!execution) {
      return route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ result: 'error', message: 'not found' }) });
    }
    execution.status = 'cancelled';
    execution.updatedAt = '2026-03-08T12:05:00Z';
    execution.results = execution.results.length
      ? execution.results
      : [{ taskId: 'task-1', status: 'failed', hostId: 'local', agentId: 'zeroclaw', error: 'cancelled', attempts: 1, latencyMs: 3 }];
    return route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify({ result: 'ok', execution }),
    });
  });

  await page.route('**/api/v1/orchestrator/executions/*', async (route) => {
    const path = new URL(route.request().url()).pathname;
    const executionID = path.split('/')[5];
    const execution = executions.find((item) => item.id === executionID);
    if (!execution) {
      return route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ result: 'error', message: 'not found' }) });
    }
    const workers = Array.isArray(execution.taskUnits)
      ? execution.taskUnits.map((task, index) => ({
        id: `worker-${index + 1}`,
        hostId: String(task.hostId || 'local'),
        agentId: String(task.agentId || 'zeroclaw'),
        state: execution.status === 'cancelled' ? 'reclaimed' : 'leased',
      }))
      : [];
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ result: 'ok', execution, workers }),
    });
  });
}

/** Login by setting token in localStorage and navigating. */
export async function loginWithToken(page: Page, url = '/') {
  await page.addInitScript((token: string) => {
    localStorage.setItem('carrier_token', token);
  }, TEST_TOKEN);
  await page.goto(url);
}
