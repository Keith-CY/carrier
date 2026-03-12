import { useMemo, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { apiGet } from '../../lib/api';
import { executionCounts, isExecutionTerminalStatus } from '../executions/ExecutionDetailContent';
import { normalizeExecutions } from './model';

export function useDashboardExecutionsData(enabled: boolean) {
  const queryClient = useQueryClient();
  const [expandedExecutionIds, setExpandedExecutionIds] = useState<Set<string>>(new Set());

  const executionsQuery = useQuery({
    queryKey: ['executions'],
    queryFn: () => apiGet<any>('/api/v1/orchestrator/executions'),
    enabled,
    refetchInterval: enabled ? 5000 : false,
  });

  const executions = useMemo(() => normalizeExecutions(executionsQuery.data), [executionsQuery.data]);
  const recentExecutions = executions.slice(0, 8);
  const activeRecentExecutions = recentExecutions.filter((item) => !isExecutionTerminalStatus(item?.status)).length;

  const refreshExecutions = async () => {
    await queryClient.invalidateQueries({ queryKey: ['executions'] });
  };

  return {
    executionsQuery,
    recentExecutions,
    activeRecentExecutions,
    expandedExecutionIds,
    refreshExecutions,
    toggleExecutionExpansion: (executionId: string) => setExpandedExecutionIds((current) => {
      const next = new Set(current);
      if (next.has(executionId)) next.delete(executionId);
      else next.add(executionId);
      return next;
    }),
    executionCounts,
  };
}
