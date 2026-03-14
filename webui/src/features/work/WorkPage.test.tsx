import type { ReactElement } from 'react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { WorkItemsPage, WorkProjectsPage, WorkRunsPage } from './WorkCollectionPages';
import { WorkItemPage } from './WorkItemPage';
import { WorkPage } from './WorkPage';
import { WorkProjectPage } from './WorkProjectPage';
import { WorkRunPage } from './WorkRunPage';

vi.mock('../../app/useFeatures', () => ({
  useFeatures: () => ({
    featureFlags: {
      remoteControlPlaneEnabled: true,
      remoteChatEnabled: true,
      providerBindingEnabled: true,
    },
    authz: {
      role: 'admin',
      permissions: {
        viewExecutions: true,
        launchExecutions: true,
        approveExecutions: true,
        managePolicies: true,
        manageProviders: true,
        manageHosts: true,
      },
    },
    isLoading: false,
  }),
}));

const projects = [
  {
    id: 'proj_alpha',
    name: 'Alpha',
    sourceType: 'github',
    sourceRef: 'git@github.com:acme/alpha.git',
    defaultBranch: 'main',
    workflowPath: 'WORKFLOW.md',
    workflowDigest: 'sha256:workflow-alpha',
    state: 'ready',
    lastSyncAt: '2026-03-14T10:00:00Z',
  },
];

const items = [
  {
    id: 'work_bug',
    projectId: 'proj_alpha',
    title: 'Fix worker drift',
    description: 'Investigate stale worker leases in the control plane.',
    acceptance: ['Document the source of drift', 'Propose remediation steps'],
    priority: 'urgent',
    source: 'github',
    sourceRef: 'issue:12',
    labels: ['sre', 'worker'],
    state: 'running',
    latestRunId: 'run_123',
    claimedByRunId: 'run_123',
    createdAt: '2026-03-13T08:00:00Z',
    updatedAt: '2026-03-14T11:05:00Z',
  },
];

const runs = [
  {
    id: 'run_123',
    projectId: 'proj_alpha',
    workItemId: 'work_bug',
    executionId: 'exec-running',
    workspaceId: 'ws_123',
    workspacePath: '/tmp/carrier/worktrees/run_123',
    backend: 'managed_isolated',
    phase: 'executing',
    leaseOwner: 'carrier:local',
    verificationStatus: 'pending',
    publishStatus: 'pending',
    workflowDigest: 'sha256:workflow-alpha',
    createdAt: '2026-03-14T11:00:00Z',
    updatedAt: '2026-03-14T11:06:00Z',
  },
];

function installFetchMocks() {
  globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url);
    const method = typeof input === 'string' || input instanceof URL ? 'GET' : String(input.method || 'GET').toUpperCase();
    if (url.endsWith('/api/v1/work/projects')) {
      if (method === 'POST') {
        return new Response(JSON.stringify({ result: 'ok', project: projects[0] }), {
          status: 201,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      return new Response(JSON.stringify({ result: 'ok', projects }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (url.endsWith('/api/v1/work/projects/proj_alpha')) {
      return new Response(JSON.stringify({ result: 'ok', project: projects[0] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (url.endsWith('/api/v1/work/projects/proj_alpha/sync')) {
      return new Response(JSON.stringify({ result: 'ok', project: projects[0] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (url.endsWith('/api/v1/work/items')) {
      if (method === 'POST') {
        return new Response(JSON.stringify({ result: 'ok', item: items[0] }), {
          status: 201,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      return new Response(JSON.stringify({ result: 'ok', items }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (url.endsWith('/api/v1/work/items/work_bug')) {
      return new Response(JSON.stringify({ result: 'ok', item: items[0] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (url.endsWith('/api/v1/work/runs')) {
      return new Response(JSON.stringify({ result: 'ok', runs }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (url.endsWith('/api/v1/work/runs/run_123')) {
      return new Response(JSON.stringify({ result: 'ok', run: runs[0] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (url.endsWith('/api/v1/work/items/work_bug/runs')) {
      return new Response(JSON.stringify({ result: 'ok', run: runs[0] }), {
        status: 201,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (url.endsWith('/api/v1/work/items/work_bug/cancel')) {
      return new Response(JSON.stringify({ result: 'ok', item: { ...items[0], state: 'cancelled' } }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (url.endsWith('/api/v1/work/items/work_bug/complete')) {
      return new Response(JSON.stringify({ result: 'ok', item: { ...items[0], state: 'done' } }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (url.endsWith('/api/v1/work/runs/run_123/resume') || url.endsWith('/api/v1/work/runs/run_123/cancel') || url.endsWith('/api/v1/work/runs/run_123/reclaim')) {
      return new Response(JSON.stringify({ result: 'ok', run: runs[0] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (url.endsWith('/api/v1/work/runs/run_123/cleanup')) {
      return new Response(JSON.stringify({ result: 'ok', cleaned: true }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (url.endsWith('/api/v1/work/adapters/github/import')) {
      return new Response(JSON.stringify({ result: 'ok', item: { ...items[0], id: 'work_imported', sourceRef: 'github:acme/alpha/issues/42' } }), {
        status: 201,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (url.endsWith('/api/v1/orchestrator/executions/exec-running')) {
      return new Response(JSON.stringify({
        result: 'ok',
        execution: {
          id: 'exec-running',
          goal: 'Investigate stale worker leases',
          status: 'running',
          updatedAt: '2026-03-14T11:06:00Z',
          work: {
            projectId: 'proj_alpha',
            workItemId: 'work_bug',
            runId: 'run_123',
            workspaceId: 'ws_123',
            workflowDigest: 'sha256:workflow-alpha',
            phase: 'executing',
          },
        },
        workers: [],
      }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    return new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json' } });
  }) as typeof fetch;
}

function renderAt(pathname: string, routePath: string, element: ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[pathname]}>
        <Routes>
          <Route path={routePath} element={element} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('Work pages', () => {
  beforeEach(() => {
    installFetchMocks();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  test('renders work landing page with project, item, and run lists', async () => {
    renderAt('/work', '/work', <WorkPage />);

    expect(await screen.findByRole('heading', { name: 'Work' })).toBeInTheDocument();
    expect(await screen.findByRole('link', { name: 'Alpha' })).toHaveAttribute('href', '/work/projects/proj_alpha');
    expect(await screen.findByRole('link', { name: 'Fix worker drift' })).toHaveAttribute('href', '/work/items/work_bug');
    expect(await screen.findByRole('link', { name: 'run_123' })).toHaveAttribute('href', '/work/runs/run_123');
    expect(screen.getByText(/ready/i)).toBeInTheDocument();
    expect(screen.getByText(/urgent/i)).toBeInTheDocument();
    expect(screen.getByText(/managed isolated/i)).toBeInTheDocument();
  });

  test('renders project detail with related items and runs', async () => {
    renderAt('/work/projects/proj_alpha', '/work/projects/:projectId', <WorkProjectPage />);

    expect(await screen.findByRole('heading', { name: 'Alpha' })).toBeInTheDocument();
    expect(screen.getByText(/git@github.com:acme\/alpha.git/i)).toBeInTheDocument();
    expect(screen.getByText(/workflow\.md/i)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Fix worker drift' })).toHaveAttribute('href', '/work/items/work_bug');
    expect(screen.getByRole('link', { name: 'run_123' })).toHaveAttribute('href', '/work/runs/run_123');
  });

  test('renders the collection routes for projects, items, and runs', async () => {
    renderAt('/work/projects', '/work/projects', <WorkProjectsPage />);
    expect(await screen.findByRole('heading', { name: 'Work Projects' })).toBeInTheDocument();
    expect(await screen.findByRole('link', { name: 'Alpha' })).toHaveAttribute('href', '/work/projects/proj_alpha');

    cleanup();
    installFetchMocks();
    renderAt('/work/items', '/work/items', <WorkItemsPage />);
    expect(await screen.findByRole('heading', { name: 'Work Items' })).toBeInTheDocument();
    expect(await screen.findByRole('link', { name: 'Fix worker drift' })).toHaveAttribute('href', '/work/items/work_bug');

    cleanup();
    installFetchMocks();
    renderAt('/work/runs', '/work/runs', <WorkRunsPage />);
    expect(await screen.findByRole('heading', { name: 'Work Runs' })).toBeInTheDocument();
    expect(await screen.findByRole('link', { name: 'run_123' })).toHaveAttribute('href', '/work/runs/run_123');
  });

  test('renders item detail with project and latest run links', async () => {
    renderAt('/work/items/work_bug', '/work/items/:itemId', <WorkItemPage />);

    expect(await screen.findByRole('heading', { name: 'Fix worker drift' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Alpha' })).toHaveAttribute('href', '/work/projects/proj_alpha');
    expect(screen.getByRole('link', { name: 'run_123' })).toHaveAttribute('href', '/work/runs/run_123');
    expect(screen.getByText(/document the source of drift/i)).toBeInTheDocument();
    expect(screen.getByText(/state/i)).toBeInTheDocument();
  });

  test('renders run detail with execution, workspace, and workflow context', async () => {
    renderAt('/work/runs/run_123', '/work/runs/:runId', <WorkRunPage />);

    expect(await screen.findByRole('heading', { name: 'run_123' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Alpha' })).toHaveAttribute('href', '/work/projects/proj_alpha');
    expect(screen.getByRole('link', { name: 'Fix worker drift' })).toHaveAttribute('href', '/work/items/work_bug');
    expect(screen.getByRole('link', { name: 'exec-running' })).toHaveAttribute('href', '/executions/exec-running');
    expect(screen.getByText(/sha256:workflow-alpha/i)).toBeInTheDocument();
    expect(screen.getByText(/\/tmp\/carrier\/worktrees\/run_123/i)).toBeInTheDocument();
  });

  test('triggers project, item, and run actions through work APIs', async () => {
    renderAt('/work/projects/proj_alpha', '/work/projects/:projectId', <WorkProjectPage />);

    await screen.findByRole('heading', { name: 'Alpha' });
    fireEvent.click(screen.getByRole('button', { name: 'Sync Project' }));
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/v1/work/projects/proj_alpha/sync', expect.objectContaining({ method: 'POST' })));

    fireEvent.change(screen.getByLabelText('Title'), { target: { value: 'Create local item' } });
    fireEvent.change(screen.getByLabelText('Description'), { target: { value: 'Document the queue contract.' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create Work Item' }));
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/v1/work/items', expect.objectContaining({ method: 'POST' })));

    fireEvent.change(screen.getByLabelText('Repository'), { target: { value: 'acme/alpha' } });
    fireEvent.change(screen.getByLabelText('Issue Number'), { target: { value: '42' } });
    fireEvent.click(screen.getByRole('button', { name: 'Import GitHub Work Item' }));
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/v1/work/adapters/github/import', expect.objectContaining({ method: 'POST' })));

    cleanup();
    installFetchMocks();
    renderAt('/work/items/work_bug', '/work/items/:itemId', <WorkItemPage />);
    await screen.findByRole('heading', { name: 'Fix worker drift' });
    fireEvent.click(screen.getByRole('button', { name: 'Start Run' }));
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/v1/work/items/work_bug/runs', expect.objectContaining({ method: 'POST' })));

    cleanup();
    installFetchMocks();
    renderAt('/work/items/work_bug', '/work/items/:itemId', <WorkItemPage />);
    await screen.findByRole('heading', { name: 'Fix worker drift' });
    fireEvent.click(screen.getByRole('button', { name: 'Resume' }));
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/v1/work/runs/run_123/resume', expect.objectContaining({ method: 'POST' })));

    cleanup();
    installFetchMocks();
    renderAt('/work/items/work_bug', '/work/items/:itemId', <WorkItemPage />);
    await screen.findByRole('heading', { name: 'Fix worker drift' });
    fireEvent.click(screen.getByRole('button', { name: 'Cancel Item' }));
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/v1/work/items/work_bug/cancel', expect.objectContaining({ method: 'POST' })));

    cleanup();
    installFetchMocks();
    renderAt('/work/items/work_bug', '/work/items/:itemId', <WorkItemPage />);
    await screen.findByRole('heading', { name: 'Fix worker drift' });
    fireEvent.click(screen.getByRole('button', { name: 'Mark Done' }));
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/v1/work/items/work_bug/complete', expect.objectContaining({ method: 'POST' })));

    cleanup();
    installFetchMocks();
    renderAt('/work/runs/run_123', '/work/runs/:runId', <WorkRunPage />);
    await screen.findByRole('heading', { name: 'run_123' });
    fireEvent.click(screen.getByRole('button', { name: 'Resume' }));
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/v1/work/runs/run_123/resume', expect.objectContaining({ method: 'POST' })));
    fireEvent.click(screen.getByRole('button', { name: 'Cleanup Workspace' }));
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledWith('/api/v1/work/runs/run_123/cleanup', expect.objectContaining({ method: 'POST' })));
  });
});
