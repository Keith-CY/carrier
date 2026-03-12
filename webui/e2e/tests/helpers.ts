import { Page } from '@playwright/test';

export const TEST_TOKEN = 'test-token-valid';
export const ADMIN_AUTHZ = {
  role: 'admin',
  permissions: {
    viewExecutions: true,
    launchExecutions: true,
    approveExecutions: true,
    managePolicies: true,
    manageProviders: true,
    manageHosts: true,
  },
};
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

export const MOCK_TEMPLATES = [
  {
    id: 'incident-diagnosis',
    name: 'Incident Diagnosis',
    description: 'Triage a live incident and produce an operator-facing diagnosis summary.',
    inputSchema: [
      { id: 'service', label: 'Service', required: true, placeholder: 'checkout' },
      { id: 'environment', label: 'Environment', required: true, placeholder: 'prod' },
      { id: 'incidentSummary', label: 'Incident Summary', required: true, placeholder: 'latency regression after deploy' },
    ],
    defaultGoalTemplate: 'Diagnose incident for service {{service}} in {{environment}}. Summary: {{incidentSummary}}.',
    plannerTasks: [
      { id: 'task-1', agentId: 'zeroclaw', inputTemplate: 'Collect incident context for {{service}} in {{environment}}.' },
      { id: 'task-2', agentId: 'picoclaw', inputTemplate: 'Analyze probable failure paths for {{service}} in {{environment}} given {{incidentSummary}}.' },
      { id: 'task-3', agentId: 'zeroclaw', inputTemplate: 'Draft diagnosis summary and operator next steps for {{service}}.' },
    ],
  },
  {
    id: 'pr-triage',
    name: 'PR Triage',
    description: 'Collect pull request context, inspect risk, and draft a recommendation.',
    inputSchema: [
      { id: 'repository', label: 'Repository', required: true, placeholder: 'Keith-CY/carrier' },
      { id: 'prNumber', label: 'PR Number', required: true, placeholder: '1554' },
      { id: 'focus', label: 'Focus', required: false, defaultValue: 'general risk assessment' },
    ],
    defaultGoalTemplate: 'Triage pull request {{repository}}#{{prNumber}} with focus on {{focus}}.',
    plannerTasks: [
      { id: 'task-1', agentId: 'zeroclaw', inputTemplate: 'Collect PR context for {{repository}}#{{prNumber}}.' },
      { id: 'task-2', agentId: 'picoclaw', inputTemplate: 'Inspect changed files and risk hotspots for {{repository}}#{{prNumber}} with focus on {{focus}}.' },
      { id: 'task-3', agentId: 'zeroclaw', inputTemplate: 'Draft a triage recommendation for {{repository}}#{{prNumber}}.' },
    ],
  },
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
    id: 'exec-ask',
    goal: 'Run picoclaw remediation on prod host',
    team: 'sre',
    project: 'checkout',
    templateId: 'incident-diagnosis',
    requestedProvider: 'openrouter',
    status: 'pending_authorization',
    updatedAt: '2026-03-09T11:40:00Z',
    authorization: {
      infrastructureApproved: false,
    },
    policy: {
      decision: 'ask',
      reason: 'picoclaw on prod hosts needs explicit review',
      matchedRuleName: 'review picoclaw production runs',
      summary: 'infrastructure approval required; tool mode restricted; effective concurrency 1',
      requiresInfrastructureApproval: true,
      configuredMaxConcurrency: 1,
      effectiveMaxConcurrency: 1,
      maxTaskTimeoutMs: 60000,
      maxRetryBudget: 0,
      toolPolicy: {
        mode: 'restricted',
        allowedTools: ['grep', 'shell'],
      },
      targets: [
        { hostId: 'host-1', agentId: 'picoclaw', count: 1 },
      ],
    },
    taskUnits: [
      { id: 'task-1', input: 'apply remediation', hostId: 'host-1', agentId: 'picoclaw' },
    ],
    results: [],
  },
  {
    id: 'exec-running',
    goal: 'Investigate checkout latency',
    team: 'platform',
    project: 'carrier',
    templateId: 'incident-diagnosis',
    triggerSource: 'schedule',
    triggerId: 'trigger-nightly',
    requestedProvider: 'anthropic',
    status: 'running',
    updatedAt: '2026-03-08T11:05:00Z',
    authorization: {
      infrastructureApproved: true,
      approvedBy: 'carrier-cli',
      approvedAt: '2026-03-08T11:01:00Z',
    },
    governance: {
      providerResolutions: [
        {
          source: 'instance',
          status: 'resolved',
          hostId: 'host-1',
          agentId: 'picoclaw',
          profileId: 'profile-anthropic',
          profileName: 'anthropic-prod',
          provider: 'anthropic',
          model: 'claude-3-7-sonnet',
          syncMode: 'manual',
          driftState: 'override',
          driftReason: 'instance binding overrides host binding',
          trace: [
            {
              source: 'instance',
              status: 'resolved',
              selected: true,
              profileId: 'profile-anthropic',
              profileName: 'anthropic-prod',
              provider: 'anthropic',
              model: 'claude-3-7-sonnet',
            },
            {
              source: 'host',
              status: 'shadowed',
              selected: false,
              profileId: 'profile-openrouter',
              profileName: 'openrouter-default',
              provider: 'openrouter',
              model: 'openai/gpt-4o-mini',
            },
          ],
        },
      ],
    },
    policy: {
      decision: 'allow',
      summary: 'infrastructure approval required; tool mode restricted; effective concurrency 2',
      requiresInfrastructureApproval: true,
      configuredMaxConcurrency: 3,
      effectiveMaxConcurrency: 2,
      maxTaskTimeoutMs: 120000,
      maxRetryBudget: 2,
      toolPolicy: {
        mode: 'restricted',
        allowedTools: ['grep', 'shell'],
      },
      targets: [
        { hostId: 'local', agentId: 'zeroclaw', count: 1 },
        { hostId: 'host-1', agentId: 'picoclaw', count: 1 },
      ],
    },
    taskUnits: [
      { id: 'task-1', input: 'collect traces', hostId: 'local', agentId: 'zeroclaw' },
      { id: 'task-2', input: 'summarize traces', hostId: 'host-1', agentId: 'picoclaw' },
    ],
    results: [
      { taskId: 'task-1', status: 'completed', hostId: 'local', agentId: 'zeroclaw', output: 'trace bundle ready', attempts: 1, latencyMs: 42 },
    ],
  },
  {
    id: 'exec-retryable',
    goal: 'Collect failing deployment evidence',
    team: 'sre',
    project: 'checkout',
    templateId: 'incident-diagnosis',
    triggerSource: 'webhook',
    triggerId: 'trigger-manual',
    requestedProvider: 'openrouter',
    status: 'retryable_failed',
    updatedAt: '2026-03-08T10:45:00Z',
    parentExecutionId: 'exec-seed-failure',
    sourceExecutionId: 'exec-seed-failure',
    launchReason: 'rerun_execution',
    authorization: {
      infrastructureApproved: true,
      approvedBy: 'carrier-cli',
      approvedAt: '2026-03-08T10:40:00Z',
    },
    policy: {
      decision: 'allow',
      summary: 'infrastructure approval required; tool mode restricted; effective concurrency 2',
      requiresInfrastructureApproval: true,
      configuredMaxConcurrency: 2,
      effectiveMaxConcurrency: 2,
      maxTaskTimeoutMs: 90000,
      maxRetryBudget: 1,
      toolPolicy: {
        mode: 'restricted',
        allowedTools: ['grep', 'shell'],
      },
      targets: [
        { hostId: 'host-1', agentId: 'picoclaw', count: 1 },
        { hostId: 'local', agentId: 'zeroclaw', count: 1 },
      ],
    },
    taskUnits: [
      { id: 'task-1', input: 'collect rollout logs', hostId: 'host-1', agentId: 'picoclaw' },
      { id: 'task-2', input: 'summarize failures', hostId: 'local', agentId: 'zeroclaw' },
    ],
    results: [
      { taskId: 'task-1', status: 'failed', hostId: 'host-1', agentId: 'picoclaw', summary: 'ssh exited with code 255', error: 'connection reset by peer', failureReason: 'remote ssh session dropped', failureCategory: 'worker_failed', attempts: 2, latencyMs: 213 },
      { taskId: 'task-2', status: 'completed', hostId: 'local', agentId: 'zeroclaw', summary: 'partial incident notes drafted', output: 'draft ready', attempts: 1, latencyMs: 22 },
    ],
    outcome: {
      summary: 'One task failed after retry budget was exhausted.',
      failureReason: 'remote evidence collection failed',
      failureCategory: 'retryable_failed',
      artifacts: [
        {
          id: 'artifact-rollout-log',
          taskId: 'task-2',
          name: 'rollout-notes.md',
          kind: 'report',
          contentType: 'text/markdown',
          sizeBytes: 512,
          path: 'artifacts/rollout-notes.md',
          createdAt: '2026-03-08T10:45:00Z',
        },
      ],
    },
  },
  {
    id: 'exec-complete',
    goal: 'Prepare release notes',
    team: 'platform',
    project: 'carrier',
    templateId: 'pr-triage',
    requestedProvider: 'openrouter',
    triggerSource: 'github',
    triggerId: 'trigger-gh-1',
    triggerEvent: 'issue_comment',
    initiator: 'github:alice',
    status: 'completed',
    updatedAt: '2026-03-08T10:30:00Z',
    parentExecutionId: 'exec-seed-release',
    sourceExecutionId: 'exec-seed-release',
    launchReason: 'clone_execution',
    authorization: {
      infrastructureApproved: true,
      approvedBy: 'webui',
      approvedAt: '2026-03-08T10:20:00Z',
    },
    governance: {
      providerResolutions: [
        {
          source: 'host',
          status: 'resolved',
          hostId: 'local',
          agentId: 'zeroclaw',
          profileId: 'profile-openrouter',
          profileName: 'openrouter-default',
          provider: 'openrouter',
          model: 'openai/gpt-4o-mini',
          syncMode: 'always_push',
          estimatedInputTokens: 40,
          estimatedOutputTokens: 28,
          estimatedTotalTokens: 68,
          estimatedCostUsd: 0.0004,
          successfulTasks: 1,
          failedTasks: 0,
          avgLatencyMs: 64,
          driftState: 'in_sync',
          trace: [
            {
              source: 'host',
              status: 'resolved',
              selected: true,
              profileId: 'profile-openrouter',
              profileName: 'openrouter-default',
              provider: 'openrouter',
              model: 'openai/gpt-4o-mini',
            },
          ],
        },
      ],
    },
    policy: {
      decision: 'allow',
      summary: 'infrastructure approval required; tool mode restricted; effective concurrency 1',
      requiresInfrastructureApproval: true,
      configuredMaxConcurrency: 1,
      effectiveMaxConcurrency: 1,
      maxTaskTimeoutMs: 60000,
      maxRetryBudget: 0,
      toolPolicy: {
        mode: 'restricted',
        allowedTools: ['grep', 'shell'],
      },
      targets: [
        { hostId: 'local', agentId: 'zeroclaw', count: 1 },
      ],
    },
    taskUnits: [
      { id: 'task-1', input: 'collect merged PRs', hostId: 'local', agentId: 'zeroclaw' },
    ],
    results: [
      { taskId: 'task-1', status: 'completed', hostId: 'local', agentId: 'zeroclaw', summary: 'release notes draft ready', output: 'release notes draft ready', attempts: 1, latencyMs: 18 },
    ],
    outcome: {
      summary: 'Release notes draft compiled and attached.',
      artifacts: [
        {
          id: 'artifact-release-notes',
          taskId: 'task-1',
          name: 'release-notes.md',
          kind: 'report',
          contentType: 'text/markdown',
          sizeBytes: 1024,
          path: 'artifacts/release-notes.md',
          createdAt: '2026-03-08T10:30:00Z',
        },
      ],
    },
  },
];

export const MOCK_WORKERS = [
  {
    id: 'local-zeroclaw',
    source: 'local',
    hostId: 'local',
    hostName: 'Local',
    agentId: 'zeroclaw',
    state: 'available',
    runtimeState: 'running',
    health: 'healthy',
    updatedAt: '2026-03-08T11:05:00Z',
  },
  {
    id: 'lease-host-1-pico',
    source: 'lease',
    hostId: 'host-1',
    hostName: 'prod-host-1',
    agentId: 'picoclaw',
    state: 'busy',
    leaseState: 'busy',
    executionId: 'exec-running',
    taskCount: 1,
    queuePosition: 1,
    stale: true,
    staleReason: 'heartbeat_timeout',
    leaseAgeSec: 920,
    heartbeatAgeSec: 360,
    lastHeartbeatAt: '2026-03-08T10:59:00Z',
    updatedAt: '2026-03-08T11:04:00Z',
  },
  {
    id: 'sync-host-1-zero',
    source: 'remote_sync',
    hostId: 'host-1',
    hostName: 'prod-host-1',
    agentId: 'zeroclaw',
    state: 'managed',
    runtimeMode: 'managed_gateway',
    health: 'healthy',
    updatedAt: '2026-03-08T11:03:00Z',
  },
];

function cloneJSON<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

export function normalizeTestRoute(url = '/'): string {
  if (!url) return '/';
  const hashIndex = url.indexOf('#');
  if (hashIndex < 0) return url;
  const hash = url.slice(hashIndex + 1).trim();
  if (!hash) return url.slice(0, hashIndex) || '/';
  if (hash.startsWith('/')) return hash;
  return '/' + hash;
}

export async function pushHistoryRoute(page: Page, url = '/') {
  const targetPath = normalizeTestRoute(url);
  await page.evaluate((nextPath: string) => {
    window.history.pushState({}, '', nextPath);
    window.dispatchEvent(new PopStateEvent('popstate', { state: window.history.state }));
  }, targetPath);
  await page.waitForTimeout(50);
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
      body: JSON.stringify({ id: 'agent-alpha', runtimeState: 'running', uptime: '1h' }),
    }),
  );

  await page.route('**/api/v1/agents/*/launcher', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        agentId: 'agent-alpha',
        status: { id: 'agent-alpha', runtimeState: 'running', health: 'healthy' },
        heartbeat: {
          state: 'fresh',
          ageSeconds: 12,
          lastActivityAt: '2026-03-12T03:59:48Z',
        },
        memory: {
          contractId: 'memory-alpha',
          contractDigest: 'sha256:abc',
        },
        providerReadiness: {
          provider: 'openrouter',
          authMode: 'api_key',
          credentialConfigured: true,
          ready: true,
        },
        cron: {
          count: 2,
          nextRunAt: '2026-03-13T00:00:00Z',
          lastRunAt: '2026-03-12T23:55:00Z',
          lastResult: 'succeeded',
          jobs: [
            { id: 'cron-1', prompt: 'check launcher', nextRunAt: '2026-03-13T00:00:00Z', lastRunAt: '2026-03-12T23:55:00Z', lastResult: 'succeeded' },
            { id: 'cron-2', prompt: 'refresh heartbeat', nextRunAt: '2026-03-13T01:00:00Z', lastResult: 'scheduled' },
          ],
        },
        session: {
          instanceId: 'instance-1',
          channel: 'telegram',
          isolation: true,
          runtimeState: 'running',
        },
        capabilities: {
          skills: [{ name: 'toolbox', enabled: true }],
          mcp: {
            servers: [{ name: 'repo', health: 'healthy', visibleToolCount: 1, hiddenToolCount: 0 }],
            visibleTools: [{ name: 'repo_search', description: 'Search code' }],
          },
        },
      }),
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

  await page.route('**/api/v1/templates', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        templates: cloneJSON(MOCK_TEMPLATES),
      }),
    }),
  );

  await page.route('**/api/v1/memory?*', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        subject: 'agent-a',
        entries: [
          { id: 'public.team.v1', type: 'public' },
          { id: 'agent-a.private.v1', type: 'per_agent' },
        ],
        attachments: [
          { agent_id: 'agent-a', memory_id: 'public.team.v1' },
          { agent_id: 'agent-a', memory_id: 'agent-a.private.v1' },
        ],
        grants: [
          { id: 'grant-1', subject: 'agent-a', scope: 'shared:profile' },
        ],
        audit: [],
      }),
    }),
  );

  await page.route('**/api/v1/memory', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        subject: '',
        entries: [
          { id: 'public.team.v1', type: 'public' },
          { id: 'agent-a.private.v1', type: 'per_agent' },
        ],
        attachments: [],
        grants: [],
        audit: [],
      }),
    }),
  );

  await page.route('**/api/v1/memory/search', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        results: [
          { id: 'rec-1', scope: 'agent:agent-a', score: 0.97, snippet: 'fusion memory' },
        ],
      }),
    }),
  );

  await page.route('**/api/v1/memory/instance/attach', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ result: 'ok', status: 'attached' }),
    }),
  );

  await page.route('**/api/v1/memory/instance/detach', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ result: 'ok', status: 'detached' }),
    }),
  );

  await page.route('**/api/v1/memory/instance/distill', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: {
          runId: 'distill-1',
          instanceId: 'picoclaw-main',
          status: 'dry_run',
          dryRun: true,
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
        hosts: [{ id: 'host-1', name: 'prod-host-1', labels: ['prod', 'gpu'] }],
      }),
    }),
  );

  await page.route('**/api/v1/provider-profiles', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        profiles: [
          {
            id: 'profile-openrouter',
            name: 'openrouter-default',
            provider: 'openrouter',
            model: 'openai/gpt-4o-mini',
            enabled: true,
          },
        ],
      }),
    }),
  );

  await page.route('**/api/v1/provider-bindings', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        bindings: [],
      }),
    }),
  );

  await page.route('**/api/v1/provider-governance/resolve*', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        resolution: {
          source: 'host',
          status: 'resolved',
          hostId: 'host-1',
          agentId: 'zeroclaw',
          profileId: 'profile-openrouter',
          profileName: 'openrouter-default',
          provider: 'openrouter',
          model: 'openai/gpt-4o-mini',
          syncMode: 'always_push',
          driftState: 'in_sync',
          trace: [
            {
              source: 'host',
              status: 'resolved',
              selected: true,
              profileId: 'profile-openrouter',
              profileName: 'openrouter-default',
              provider: 'openrouter',
              model: 'openai/gpt-4o-mini',
            },
          ],
        },
      }),
    }),
  );

  await page.route('**/api/v1/triggers', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        triggers: [],
      }),
    }),
  );

  await page.route('**/api/v1/triggers/*', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        trigger: {
          id: 'trigger-1',
          name: 'default trigger',
          type: 'webhook',
          templateId: 'incident-diagnosis',
          enabled: true,
          config: {
            webhookSecretConfigured: true,
          },
        },
      }),
    }),
  );

  await page.route('**/api/v1/orchestrator/policies', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        policies: [],
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

  await page.route('**/api/v1/orchestrator/workers', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        workers: cloneJSON(MOCK_WORKERS),
        summary: {
          total: MOCK_WORKERS.length,
          active: 1,
          busy: 1,
          error: 0,
          local: 1,
          remote: 2,
        },
        warnings: [],
      }),
    }),
  );

  await page.route('**/api/v1/orchestrator/workers/reclaim', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        reclaim: {
          reclaimed: 1,
          skipped: 1,
          failed: 0,
          failures: [],
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
        authz: cloneJSON(ADMIN_AUTHZ),
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
  const workers = cloneJSON(MOCK_WORKERS);
  let nextExecutionID = 1;
  let nextDerivedExecutionID = 1;

  function queueSummary() {
    return {
      activeExecutions: executions.filter((item) => ['running', 'provisioning'].includes(String(item.status || '').trim().toLowerCase())).length,
      queuedTasks: 2,
      staleLeases: workers.filter((item) => item.source === 'lease' && item.stale).length,
      reclaimableWorkers: workers.filter((item) => item.source === 'lease' && item.stale).length,
      updatedAt: '2026-03-08T11:05:00Z',
    };
  }

  function deriveExecution(source: Record<string, unknown>, action: 'retry' | 'rerun' | 'clone') {
    const nextID = `exec-derived-${nextDerivedExecutionID++}`;
    const sourceExecutionID = String(source.sourceExecutionId || source.id || '').trim();
    const policy = source.policy && typeof source.policy === 'object'
      ? cloneJSON(source.policy as Record<string, unknown>)
      : undefined;
    if (policy) {
      delete policy.approvedBy;
      delete policy.approvedAt;
    }
    let taskUnits = Array.isArray(source.taskUnits)
      ? cloneJSON(source.taskUnits as Array<Record<string, unknown>>)
      : [];
    if (action === 'retry') {
      const failedTaskIDs = new Set(
        (Array.isArray(source.results) ? source.results : [])
          .filter((item) => String((item as Record<string, unknown>).status || '').trim().toLowerCase() === 'failed')
          .map((item) => String((item as Record<string, unknown>).taskId || '').trim())
          .filter(Boolean),
      );
      taskUnits = taskUnits.filter((task) => failedTaskIDs.has(String(task.id || '').trim()));
      if (!taskUnits.length) return null;
    }
    const derived = {
      ...cloneJSON(source),
      id: nextID,
      status: 'pending_authorization',
      parentExecutionId: String(source.id || '').trim(),
      sourceExecutionId: sourceExecutionID || String(source.id || '').trim(),
      launchReason: action === 'retry' ? 'retry_failed_tasks' : action === 'rerun' ? 'rerun_execution' : 'clone_execution',
      updatedAt: '2026-03-09T12:10:00Z',
      authorization: {
        infrastructureApproved: false,
      },
      policy,
      taskUnits,
      results: [],
      outcome: {},
      error: '',
    };
    executions.unshift(derived);
    return derived;
  }

  await page.route('**/api/v1/orchestrator/plans', async (route) => {
    const body = route.request().postDataJSON() as Record<string, unknown>;
    const templateID = String(body.templateId || '').trim();
    const templateInputs = body.inputs && typeof body.inputs === 'object'
      ? (body.inputs as Record<string, unknown>)
      : {};
    const goal = String(body.goal || '').trim();
    const provider = String(body.provider || '').trim();
    const hostIds = Array.isArray(body.hostIds) ? (body.hostIds as string[]) : [];
    const hostLabels = Array.isArray(body.hostLabels) ? (body.hostLabels as string[]) : [];
    const maxConcurrency = Number(body.maxConcurrency || 0) || 2;
    const firstHostId = hostIds[0] || '';
    const secondHostId = hostIds[1] || hostIds[0] || '';
    const workerSelector = hostLabels.length
      ? { hostLabels }
      : { hostId: firstHostId || 'local' };
    const secondWorkerSelector = hostLabels.length
      ? { hostLabels }
      : { hostId: secondHostId || 'local' };
    const renderedGoal = templateID === 'incident-diagnosis'
      ? `Diagnose incident for service ${String(templateInputs.service || '').trim()} in ${String(templateInputs.environment || '').trim()}. Summary: ${String(templateInputs.incidentSummary || '').trim()}.`
      : goal;
    const plannerTasks = templateID === 'incident-diagnosis'
      ? [
          { id: 'task-1', input: `Collect incident context for ${String(templateInputs.service || '').trim()} in ${String(templateInputs.environment || '').trim()}.`, agentId: 'zeroclaw' },
          { id: 'task-2', input: `Analyze probable failure paths for ${String(templateInputs.service || '').trim()} in ${String(templateInputs.environment || '').trim()} given ${String(templateInputs.incidentSummary || '').trim()}.`, agentId: 'picoclaw' },
          { id: 'task-3', input: `Draft diagnosis summary and operator next steps for ${String(templateInputs.service || '').trim()}.`, agentId: 'zeroclaw' },
        ]
      : [
          { id: 'task-1', input: 'collect context', agentId: 'zeroclaw' },
          { id: 'task-2', input: 'draft summary', agentId: 'picoclaw' },
        ];
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        plan: {
          goal: renderedGoal,
          templateId: templateID,
          provider,
          hostIds,
          hostLabels,
          approvalScope: 'infrastructure_only',
          maxConcurrency,
          plannerTasks,
          requiredWorkers: [
            { ...workerSelector, agentId: 'zeroclaw', count: 1 },
            { ...secondWorkerSelector, agentId: 'picoclaw', count: 1 },
          ],
          taskUnits: plannerTasks.map((task, index) => ({
            id: String(task.id),
            input: String(task.input),
            ...(index === 0 ? workerSelector : secondWorkerSelector),
            agentId: String(task.agentId),
            timeoutMs: 60000,
            retryBudget: 0,
          })),
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
    const taskUnits = cloneJSON((body.taskUnits as Array<Record<string, unknown>>) || []);
    const configuredMaxConcurrency = Number(body.maxConcurrency || 0) || taskUnits.length || 1;
    const effectiveMaxConcurrency = taskUnits.length
      ? Math.min(configuredMaxConcurrency, taskUnits.length)
      : configuredMaxConcurrency;
    const execution = {
      id: `exec-preview-${nextExecutionID++}`,
      goal: String(body.goal || '').trim(),
      templateId: String(body.templateId || '').trim(),
      requestedProvider: String(body.requestedProvider || '').trim(),
      status: 'pending_authorization',
      updatedAt: '2026-03-08T12:00:00Z',
      authorization: {
        infrastructureApproved: false,
      },
      governance: {
        providerResolutions: [
          {
            source: 'host',
            status: 'resolved',
            hostId: 'local',
            agentId: 'zeroclaw',
            profileId: 'profile-openrouter',
            profileName: 'openrouter-default',
            provider: 'openrouter',
            model: 'openai/gpt-4o-mini',
            syncMode: 'always_push',
          },
        ],
      },
      policy: {
        decision: 'allow',
        summary: 'infrastructure approval required; tool mode restricted; effective concurrency ' + String(effectiveMaxConcurrency),
        requiresInfrastructureApproval: true,
        configuredMaxConcurrency,
        effectiveMaxConcurrency,
        maxTaskTimeoutMs: taskUnits.reduce((max, task) => Math.max(max, Number(task.timeoutMs || 0) || 0), 0),
        maxRetryBudget: taskUnits.reduce((max, task) => Math.max(max, Number(task.retryBudget || 0) || 0), 0),
        toolPolicy: {
          mode: 'restricted',
          allowedTools: ['grep', 'shell'],
        },
        targets: Array.isArray(body.requiredWorkers) ? cloneJSON(body.requiredWorkers as Array<Record<string, unknown>>) : [],
      },
      taskUnits,
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
    const body = route.request().postDataJSON() as Record<string, unknown>;
    if (execution.id === 'exec-ask' && !body.policyApproved) {
      return route.fulfill({
        status: 409,
        contentType: 'application/json',
        body: JSON.stringify({
          error: {
            code: 'E_POLICY_APPROVAL_REQUIRED',
            message: 'policy approval required before execution can run',
          },
          execution,
        }),
      });
    }
    execution.status = 'running';
    execution.updatedAt = '2026-03-08T12:01:00Z';
    execution.authorization = {
      infrastructureApproved: true,
      approvedBy: 'webui',
      approvedAt: '2026-03-08T12:01:00Z',
    };
    if (body.policyApproved && execution.policy) {
      execution.policy.approvedBy = 'webui';
      execution.policy.approvedAt = '2026-03-09T11:42:00Z';
    }
    if (execution.id === 'exec-ask') {
      execution.status = 'provisioning';
      execution.updatedAt = '2026-03-09T11:42:00Z';
    }
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

  await page.route('**/api/v1/orchestrator/executions/*/retry', async (route) => {
    const path = new URL(route.request().url()).pathname;
    const executionID = path.split('/')[5];
    const execution = executions.find((item) => item.id === executionID);
    if (!execution) {
      return route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ result: 'error', message: 'not found' }) });
    }
    const derived = deriveExecution(execution as Record<string, unknown>, 'retry');
    if (!derived) {
      return route.fulfill({
        status: 409,
        contentType: 'application/json',
        body: JSON.stringify({
          result: 'error',
          error: {
            code: 'E_ORCHESTRATOR_RETRY_NOTHING',
            message: 'execution has no failed tasks to retry',
          },
        }),
      });
    }
    return route.fulfill({
      status: 201,
      contentType: 'application/json',
      body: JSON.stringify({ result: 'ok', execution: derived }),
    });
  });

  await page.route('**/api/v1/orchestrator/executions/*/rerun', async (route) => {
    const path = new URL(route.request().url()).pathname;
    const executionID = path.split('/')[5];
    const execution = executions.find((item) => item.id === executionID);
    if (!execution) {
      return route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ result: 'error', message: 'not found' }) });
    }
    const derived = deriveExecution(execution as Record<string, unknown>, 'rerun');
    return route.fulfill({
      status: 201,
      contentType: 'application/json',
      body: JSON.stringify({ result: 'ok', execution: derived }),
    });
  });

  await page.route('**/api/v1/orchestrator/executions/*/clone', async (route) => {
    const path = new URL(route.request().url()).pathname;
    const executionID = path.split('/')[5];
    const execution = executions.find((item) => item.id === executionID);
    if (!execution) {
      return route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ result: 'error', message: 'not found' }) });
    }
    const derived = deriveExecution(execution as Record<string, unknown>, 'clone');
    return route.fulfill({
      status: 201,
      contentType: 'application/json',
      body: JSON.stringify({ result: 'ok', execution: derived }),
    });
  });

  await page.route('**/api/v1/orchestrator/executions/*/artifacts/*', async (route) => {
    const path = new URL(route.request().url()).pathname;
    const segments = path.split('/');
    const executionID = segments[5];
    const artifactID = segments[7];
    const execution = executions.find((item) => item.id === executionID);
    const artifacts = Array.isArray(execution && execution.outcome && (execution.outcome as Record<string, unknown>).artifacts)
      ? ((execution.outcome as Record<string, unknown>).artifacts as Array<Record<string, unknown>>)
      : [];
    const artifact = artifacts.find((item) => String(item.id || '').trim() === artifactID);
    if (!artifact) {
      return route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ result: 'error', message: 'not found' }) });
    }
    return route.fulfill({
      status: 200,
      contentType: String(artifact.contentType || 'text/plain'),
      body: '# artifact ' + String(artifact.name || artifact.id || ''),
    });
  });

  await page.route('**/api/v1/orchestrator/executions/*/evidence?format=zip', async (route) => {
    const path = new URL(route.request().url()).pathname;
    const executionID = path.split('/')[5];
    const execution = executions.find((item) => item.id === executionID);
    if (!execution) {
      return route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ result: 'error', message: 'not found' }) });
    }
    return route.fulfill({
      status: 200,
      contentType: 'application/zip',
      body: 'PK\x03\x04mock-evidence-' + executionID,
    });
  });

  await page.route('**/api/v1/audit/export?executionId=*', async (route) => {
    const executionID = new URL(route.request().url()).searchParams.get('executionId') || '';
    const execution = executions.find((item) => item.id === executionID);
    if (!execution) {
      return route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ result: 'error', message: 'not found' }) });
    }
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        executionId: executionID,
        events: [
          { action: 'orchestrator_execution_create', target: executionID, result: 'ok' },
          { action: 'gateway_audit_export', target: executionID, result: 'ok' },
        ],
      }),
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

  await page.route('**/api/v1/orchestrator/workers', async (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        workers,
        summary: {
          total: workers.length,
          active: workers.filter((item) => ['busy', 'provisioning', 'reclaiming'].includes(String(item.state))).length,
          busy: workers.filter((item) => String(item.state) === 'busy').length,
          error: workers.filter((item) => String(item.state) === 'error').length,
          local: workers.filter((item) => String(item.hostId) === 'local').length,
          remote: workers.filter((item) => String(item.hostId) !== 'local').length,
          stale: workers.filter((item) => !!item.stale).length,
        },
        warnings: [],
      }),
    }),
  );

  await page.route('**/api/v1/orchestrator/workers/queue', async (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        summary: queueSummary(),
      }),
    }),
  );

  await page.route('**/api/v1/orchestrator/workers/reclaim', async (route) => {
    for (const worker of workers) {
      if (worker.source === 'lease' && worker.state !== 'reclaimed') {
        worker.state = 'reclaimed';
        worker.leaseState = 'reclaimed';
        worker.stale = false;
        worker.staleReason = '';
      }
    }
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        reclaim: {
          reclaimed: 1,
          skipped: 0,
          failed: 0,
          failures: [],
        },
      }),
    });
  });

  await page.route('**/api/v1/orchestrator/workers/reclaim-stale', async (route) => {
    let reclaimed = 0;
    for (const worker of workers) {
      if (worker.source === 'lease' && worker.stale) {
        worker.state = 'reclaimed';
        worker.leaseState = 'reclaimed';
        worker.stale = false;
        worker.staleReason = '';
        reclaimed += 1;
      }
    }
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        reclaim: {
          reclaimed,
          skipped: 0,
          failed: 0,
          failures: [],
        },
      }),
    });
  });
}

/** Login by setting token in localStorage and navigating. */
export async function loginWithToken(page: Page, url = '/', waitUntil: 'load' | 'domcontentloaded' | 'commit' = 'commit') {
  await page.addInitScript((token: string) => {
    localStorage.setItem('carrier_token', token);
  }, TEST_TOKEN);
  const targetPath = normalizeTestRoute(url);
  await page.goto('/', { waitUntil });
  await page.locator('#logout-btn').waitFor({ state: 'visible' });
  if (targetPath !== '/') {
    await pushHistoryRoute(page, targetPath);
  }
  await page.waitForFunction((expectedPath: string) => {
    const nav = document.querySelector('#nav');
    const current = window.location.pathname || '/';
    return current === expectedPath || (!!nav && !nav.classList.contains('hidden'));
  }, targetPath);
}
