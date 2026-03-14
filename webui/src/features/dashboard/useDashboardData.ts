import { useQuery } from '@tanstack/react-query';
import { apiGet } from '../../lib/api';
import { useDashboardCapabilities } from './useDashboardCapabilities';
import { useDashboardExecutionsData } from './useDashboardExecutionsData';
import { useDashboardInstancesData } from './useDashboardInstancesData';
import { useQuickLaunchData } from './useQuickLaunchData';

export function useDashboardData() {
  const capabilities = useDashboardCapabilities();
  const instancesData = useDashboardInstancesData();
  const executionsData = useDashboardExecutionsData(capabilities.canViewExecutions);
  const quickLaunchData = useQuickLaunchData(capabilities.canLaunchExecutions);

  return {
    featureFlags: capabilities.featureFlags,
    authz: capabilities.authz,
    ...instancesData,
    ...executionsData,
    ...quickLaunchData,
  };
}

export function useDashboardExecutionDetail(executionId: string) {
  return useQuery({
    queryKey: ['execution-detail', executionId],
    queryFn: () => apiGet<any>(`/api/v1/orchestrator/executions/${encodeURIComponent(executionId)}`),
    enabled: !!executionId,
  });
}

export type DashboardData = ReturnType<typeof useDashboardData>;
