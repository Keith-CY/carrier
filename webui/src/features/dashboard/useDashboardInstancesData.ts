import { useMemo, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { apiGet, apiPost } from '../../lib/api';
import { normalizeAgentCatalog, normalizeInstances } from './model';

export function useDashboardInstancesData() {
  const queryClient = useQueryClient();
  const [addAgentModalOpen, setAddAgentModalOpen] = useState(false);

  const instancesQuery = useQuery({
    queryKey: ['instances'],
    queryFn: () => apiGet<any>('/api/v1/instances'),
  });
  const agentCatalogQuery = useQuery({
    queryKey: ['agent-catalog'],
    queryFn: () => apiGet<any>('/api/v1/agents'),
    enabled: addAgentModalOpen,
  });

  const instances = useMemo(() => normalizeInstances(instancesQuery.data), [instancesQuery.data]);
  const agentCatalog = useMemo(() => normalizeAgentCatalog(agentCatalogQuery.data), [agentCatalogQuery.data]);

  const refreshInstances = async () => {
    await queryClient.invalidateQueries({ queryKey: ['instances'] });
  };

  const handleInstanceAction = async (instanceId: string, action: string) => {
    if (action === 'uninstall' && !window.confirm(`Uninstall instance ${instanceId}?`)) return;
    try {
      await apiPost(`/api/v1/instances/${encodeURIComponent(instanceId)}/${action}`, {});
      await refreshInstances();
    } catch {
      // Keep UI responsive. Summary error line is sufficient for now.
    }
  };

  const runningInstances = instances.filter((item) => {
    const runtime = String(item?.runtime_state || item?.runtimeState || item?.runtime || '').trim().toLowerCase();
    return runtime === 'running' || runtime === 'healthy';
  }).length;

  return {
    instancesQuery,
    agentCatalogQuery,
    addAgentModalOpen,
    setAddAgentModalOpen,
    agentCatalog,
    instances,
    runningInstances,
    refreshInstances,
    handleInstanceAction,
  };
}
