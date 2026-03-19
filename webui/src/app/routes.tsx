import type { ComponentType } from 'react';
import { createBrowserRouter, Navigate, type RouteObject } from 'react-router-dom';
import { AppLayout } from './AppLayout';
import { FeatureGate } from './FeatureGate';

function lazyPage<TModule extends Record<string, unknown>, TKey extends keyof TModule>(
  load: () => Promise<TModule>,
  exportName: TKey,
) {
  return async () => {
    const module = await load();
    return {
      Component: module[exportName] as ComponentType,
    };
  };
}

function lazyPageWithProps<TModule extends Record<string, unknown>, TKey extends keyof TModule>(
  load: () => Promise<TModule>,
  exportName: TKey,
  props: Record<string, unknown>,
) {
  return async () => {
    const module = await load();
    const Component = module[exportName] as ComponentType<Record<string, unknown>>;
    return {
      Component: () => <Component {...props} />,
    };
  };
}

function lazyGatedPage<TModule extends Record<string, unknown>, TKey extends keyof TModule>(
  load: () => Promise<TModule>,
  exportName: TKey,
  gate: { requireRemoteControlPlane?: boolean; requireRemoteChat?: boolean },
  props?: Record<string, unknown>,
) {
  return async () => {
    const module = await load();
    const Component = module[exportName] as ComponentType<Record<string, unknown>>;
    return {
      Component: () => (
        <FeatureGate {...gate}>
          <Component {...(props || {})} />
        </FeatureGate>
      ),
    };
  };
}

export const routeObjects: RouteObject[] = [
  {
    path: '/',
    element: <AppLayout />,
    children: [
      {
        index: true,
        element: <Navigate to="/home" replace />,
      },
      {
        path: 'onboarding',
        lazy: lazyPage(() => import('../features/onboarding/OnboardingHubPage'), 'OnboardingHubPage'),
      },
      {
        path: 'welcome',
        lazy: lazyPageWithProps(() => import('../features/onboarding/OnboardingPage'), 'OnboardingPage', { step: 'welcome' }),
      },
      {
        path: 'setup',
        lazy: lazyPageWithProps(() => import('../features/onboarding/OnboardingPage'), 'OnboardingPage', { step: 'setup' }),
      },
      {
        path: 'provider',
        lazy: lazyPageWithProps(() => import('../features/onboarding/OnboardingPage'), 'OnboardingPage', { step: 'provider' }),
      },
      {
        path: 'config',
        lazy: lazyPageWithProps(() => import('../features/onboarding/OnboardingPage'), 'OnboardingPage', { step: 'config' }),
      },
      {
        path: 'install',
        lazy: lazyPageWithProps(() => import('../features/onboarding/OnboardingPage'), 'OnboardingPage', { step: 'install' }),
      },
      {
        path: 'complete',
        lazy: lazyPageWithProps(() => import('../features/onboarding/OnboardingPage'), 'OnboardingPage', { step: 'complete' }),
      },
      {
        path: 'add/:agentId',
        lazy: lazyPageWithProps(() => import('../features/onboarding/OnboardingPage'), 'OnboardingPage', { step: 'setup' }),
      },
      {
        path: 'home',
        lazy: lazyPage(() => import('../features/chat/ChatPage'), 'ChatPage'),
      },
      {
        path: 'chat',
        lazy: lazyPage(() => import('../features/chat/ChatPage'), 'ChatPage'),
      },
      {
        path: 'dashboard',
        lazy: lazyGatedPage(() => import('../features/dashboard/DashboardPage'), 'DashboardPage', {
          requireRemoteControlPlane: true,
        }),
      },
      {
        path: 'quick-entry',
        lazy: lazyPage(() => import('../features/quick-entry/QuickEntryPage'), 'QuickEntryPage'),
      },
      {
        path: 'projects',
        lazy: lazyPage(() => import('../features/work/ProjectsPage'), 'ProjectsPage'),
      },
      {
        path: 'projects/:projectId',
        lazy: lazyPage(() => import('../features/work/ProjectDetailPage'), 'ProjectDetailPage'),
      },
      {
        path: 'agents',
        lazy: lazyPage(() => import('../features/agents/AgentsPage'), 'AgentsPage'),
      },
      {
        path: 'agents/:agentId',
        lazy: lazyPage(() => import('../features/agents/AgentDetailPage'), 'AgentDetailPage'),
      },
      {
        path: 'agent-detail/:agentId',
        lazy: lazyPage(() => import('../features/agents/AgentDetailPage'), 'AgentDetailPage'),
      },
      {
        path: 'activity',
        lazy: lazyPage(() => import('../features/activity/ActivityPage'), 'ActivityPage'),
      },
      {
        path: 'executions',
        lazy: lazyGatedPage(() => import('../features/executions/ExecutionsPage'), 'ExecutionsPage', {
          requireRemoteControlPlane: true,
        }),
      },
      {
        path: 'executions/:executionId',
        lazy: lazyGatedPage(() => import('../features/executions/ExecutionsPage'), 'ExecutionsPage', {
          requireRemoteControlPlane: true,
        }),
      },
      {
        path: 'work',
        lazy: lazyGatedPage(() => import('../features/work/WorkPage'), 'WorkPage', {
          requireRemoteControlPlane: true,
        }),
      },
      {
        path: 'work/projects',
        lazy: lazyGatedPage(() => import('../features/work/WorkProjectPage'), 'WorkProjectPage', {
          requireRemoteControlPlane: true,
        }),
      },
      {
        path: 'work/projects/:projectId',
        lazy: lazyGatedPage(() => import('../features/work/WorkProjectPage'), 'WorkProjectPage', {
          requireRemoteControlPlane: true,
        }),
      },
      {
        path: 'work/items',
        lazy: lazyGatedPage(() => import('../features/work/WorkItemPage'), 'WorkItemPage', {
          requireRemoteControlPlane: true,
        }),
      },
      {
        path: 'work/items/:itemId',
        lazy: lazyGatedPage(() => import('../features/work/WorkItemPage'), 'WorkItemPage', {
          requireRemoteControlPlane: true,
        }),
      },
      {
        path: 'work/runs',
        lazy: lazyGatedPage(() => import('../features/work/WorkRunPage'), 'WorkRunPage', {
          requireRemoteControlPlane: true,
        }),
      },
      {
        path: 'work/runs/:runId',
        lazy: lazyGatedPage(() => import('../features/work/WorkRunPage'), 'WorkRunPage', {
          requireRemoteControlPlane: true,
        }),
      },
      {
        path: 'memory',
        lazy: lazyPage(() => import('../features/memory/MemoryPage'), 'MemoryPage'),
      },
      {
        path: 'logs',
        lazy: lazyPage(() => import('../features/logs/LogsPage'), 'LogsPage'),
      },
      {
        path: 'workers',
        lazy: lazyGatedPage(() => import('../features/workers/WorkersPage'), 'WorkersPage', {
          requireRemoteControlPlane: true,
        }),
      },
      {
        path: 'hosts',
        lazy: lazyGatedPage(() => import('../features/hosts/HostsPage'), 'HostsPage', {
          requireRemoteControlPlane: true,
        }),
      },
      {
        path: 'providers',
        lazy: lazyGatedPage(() => import('../features/providers/ProvidersPage'), 'ProvidersPage', {
          requireRemoteControlPlane: true,
        }),
      },
      {
        path: 'policies',
        lazy: lazyGatedPage(() => import('../features/policies/PoliciesPage'), 'PoliciesPage', {
          requireRemoteControlPlane: true,
        }),
      },
      {
        path: 'remote-observability',
        lazy: lazyGatedPage(() => import('../features/observability/ObservabilityPage'), 'ObservabilityPage', {
          requireRemoteControlPlane: true,
        }),
      },
      {
        path: 'remote-chat',
        lazy: lazyGatedPage(() => import('../features/remote-chat/RemoteChatPage'), 'RemoteChatPage', {
          requireRemoteChat: true,
        }),
      },
      {
        path: 'settings',
        lazy: lazyPage(() => import('../features/settings/SettingsHubPage'), 'SettingsHubPage'),
      },
      {
        path: '*',
        element: <Navigate to="/home" replace />,
      },
    ],
  },
];

export function createAppRouter() {
  return createBrowserRouter(routeObjects);
}
