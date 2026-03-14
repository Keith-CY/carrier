import { createBrowserRouter, Navigate, type RouteObject } from 'react-router-dom';
import { AppLayout } from './AppLayout';
import { FeatureGate } from './FeatureGate';
import { DashboardPage } from '../features/dashboard/DashboardPage';
import { ExecutionsPage } from '../features/executions/ExecutionsPage';
import { MemoryPage } from '../features/memory/MemoryPage';
import { WorkersPage } from '../features/workers/WorkersPage';
import { HostsPage } from '../features/hosts/HostsPage';
import { ProvidersPage } from '../features/providers/ProvidersPage';
import { PoliciesPage } from '../features/policies/PoliciesPage';
import { ObservabilityPage } from '../features/observability/ObservabilityPage';
import { SettingsPage } from '../features/settings/SettingsPage';
import { LogsPage } from '../features/logs/LogsPage';
import { ChatPage } from '../features/chat/ChatPage';
import { RemoteChatPage } from '../features/remote-chat/RemoteChatPage';
import { OnboardingPage } from '../features/onboarding/OnboardingPage';
import { AgentDetailPage } from '../features/agents/AgentDetailPage';
import { WorkPage } from '../features/work/WorkPage';
import { WorkItemsPage, WorkProjectsPage, WorkRunsPage } from '../features/work/WorkCollectionPages';
import { WorkProjectPage } from '../features/work/WorkProjectPage';
import { WorkItemPage } from '../features/work/WorkItemPage';
import { WorkRunPage } from '../features/work/WorkRunPage';

export const routeObjects: RouteObject[] = [
  {
    path: '/',
    element: <AppLayout />,
    children: [
      {
        index: true,
        element: <Navigate to="/welcome" replace />,
      },
      {
        path: 'dashboard',
        element: <DashboardPage />,
      },
      {
        path: 'welcome',
        element: <OnboardingPage step="welcome" />,
      },
      {
        path: 'setup',
        element: <OnboardingPage step="setup" />,
      },
      {
        path: 'agents/:agentId',
        element: <AgentDetailPage />,
      },
      {
        path: 'agents',
        element: <OnboardingPage step="agents" />,
      },
      {
        path: 'provider',
        element: <OnboardingPage step="provider" />,
      },
      {
        path: 'config',
        element: <OnboardingPage step="config" />,
      },
      {
        path: 'install',
        element: <OnboardingPage step="install" />,
      },
      {
        path: 'complete',
        element: <OnboardingPage step="complete" />,
      },
      {
        path: 'add/:agentId',
        element: <OnboardingPage step="setup" />,
      },
      {
        path: 'executions',
        element: <FeatureGate requireRemoteControlPlane><ExecutionsPage /></FeatureGate>,
      },
      {
        path: 'work',
        element: <FeatureGate requireRemoteControlPlane><WorkPage /></FeatureGate>,
      },
      {
        path: 'work/projects',
        element: <FeatureGate requireRemoteControlPlane><WorkProjectsPage /></FeatureGate>,
      },
      {
        path: 'work/projects/:projectId',
        element: <FeatureGate requireRemoteControlPlane><WorkProjectPage /></FeatureGate>,
      },
      {
        path: 'work/items',
        element: <FeatureGate requireRemoteControlPlane><WorkItemsPage /></FeatureGate>,
      },
      {
        path: 'work/items/:itemId',
        element: <FeatureGate requireRemoteControlPlane><WorkItemPage /></FeatureGate>,
      },
      {
        path: 'work/runs',
        element: <FeatureGate requireRemoteControlPlane><WorkRunsPage /></FeatureGate>,
      },
      {
        path: 'work/runs/:runId',
        element: <FeatureGate requireRemoteControlPlane><WorkRunPage /></FeatureGate>,
      },
      {
        path: 'executions/:executionId',
        element: <FeatureGate requireRemoteControlPlane><ExecutionsPage /></FeatureGate>,
      },
      {
        path: 'memory',
        element: <MemoryPage />,
      },
      {
        path: 'workers',
        element: <FeatureGate requireRemoteControlPlane><WorkersPage /></FeatureGate>,
      },
      {
        path: 'hosts',
        element: <FeatureGate requireRemoteControlPlane><HostsPage /></FeatureGate>,
      },
      {
        path: 'providers',
        element: <FeatureGate requireRemoteControlPlane><ProvidersPage /></FeatureGate>,
      },
      {
        path: 'policies',
        element: <FeatureGate requireRemoteControlPlane><PoliciesPage /></FeatureGate>,
      },
      {
        path: 'remote-observability',
        element: <FeatureGate requireRemoteControlPlane><ObservabilityPage /></FeatureGate>,
      },
      {
        path: 'settings',
        element: <SettingsPage />,
      },
      {
        path: 'logs',
        element: <LogsPage />,
      },
      {
        path: 'chat',
        element: <ChatPage />,
      },
      {
        path: 'remote-chat',
        element: <FeatureGate requireRemoteChat><RemoteChatPage /></FeatureGate>,
      },
      {
        path: 'agent-detail/:agentId',
        element: <AgentDetailPage />,
      },
      {
        path: '*',
        element: <Navigate to="/welcome" replace />,
      },
    ],
  },
];

export function createAppRouter() {
  return createBrowserRouter(routeObjects);
}
