import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { apiGet, apiPost } from '../../lib/api';
import { useFeatures } from '../../app/useFeatures';
import {
  executionCounts,
  executionHasFailedTasks,
  executionTemplateValue,
  executionTriggerValue,
  isExecutionTerminalStatus,
} from './ExecutionDetailContent';
import { filterExecutions, normalizeExecutions, selectExecutionId } from './model';

export function useExecutionsData() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { featureFlags, authz } = useFeatures();
  const { executionId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const [selectedExecutionId, setSelectedExecutionId] = useState(String(executionId || '').trim());

  const searchValue = searchParams.get('search') || '';
  const statusFilter = searchParams.get('status') || 'all';
  const templateFilter = searchParams.get('template') || 'all';
  const triggerFilter = searchParams.get('trigger') || 'all';

  const executionsQuery = useQuery({
    queryKey: ['executions'],
    queryFn: () => apiGet<any>('/api/v1/orchestrator/executions'),
    enabled: featureFlags.remoteControlPlaneEnabled && authz.permissions.viewExecutions,
    refetchInterval: featureFlags.remoteControlPlaneEnabled && authz.permissions.viewExecutions ? 5000 : false,
  });

  const detailQuery = useQuery({
    queryKey: ['execution-detail', selectedExecutionId],
    queryFn: () => apiGet<any>(`/api/v1/orchestrator/executions/${encodeURIComponent(selectedExecutionId)}`),
    enabled: !!selectedExecutionId,
  });

  const executions = useMemo(() => normalizeExecutions(executionsQuery.data), [executionsQuery.data]);
  const filteredExecutions = useMemo(() => filterExecutions(executions, {
    searchValue,
    statusFilter,
    templateFilter,
    triggerFilter,
  }), [executions, searchValue, statusFilter, templateFilter, triggerFilter]);

  const templateOptions = useMemo(
    () => [...new Set(executions.map((item) => executionTemplateValue(item)).filter(Boolean))].sort((left, right) => left.localeCompare(right)),
    [executions],
  );
  const triggerOptions = useMemo(
    () => [...new Set(executions.map((item) => executionTriggerValue(item)).filter(Boolean))].sort((left, right) => left.localeCompare(right)),
    [executions],
  );

  useEffect(() => {
    setSelectedExecutionId(selectExecutionId({
      routeExecutionId: String(executionId || '').trim(),
      selectedExecutionId,
      filteredExecutions,
    }));
  }, [executionId, filteredExecutions, selectedExecutionId]);

  const invalidateExecutionData = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['executions'] }),
      selectedExecutionId ? queryClient.invalidateQueries({ queryKey: ['execution-detail', selectedExecutionId] }) : Promise.resolve(),
    ]);
  };

  const cancelMutation = useMutation({
    mutationFn: () => apiPost(`/api/v1/orchestrator/executions/${encodeURIComponent(selectedExecutionId)}/cancel`, {
      actor: 'webui',
    }),
    onSuccess: async () => {
      await invalidateExecutionData();
    },
  });

  const approveMutation = useMutation({
    mutationFn: () => apiPost(`/api/v1/orchestrator/executions/${encodeURIComponent(selectedExecutionId)}/authorize`, {
      approved: true,
      actor: 'webui',
      policyApproved: true,
    }),
    onSuccess: async () => {
      await invalidateExecutionData();
    },
  });

  const derivedMutation = useMutation({
    mutationFn: async (action: 'retry' | 'rerun' | 'clone') => {
      const payload = await apiPost<any>(`/api/v1/orchestrator/executions/${encodeURIComponent(selectedExecutionId)}/${action}`);
      return payload?.execution || null;
    },
    onSuccess: async (execution) => {
      await queryClient.invalidateQueries({ queryKey: ['executions'] });
      const derivedId = String(execution?.id || '').trim();
      if (derivedId) navigate(`/executions/${encodeURIComponent(derivedId)}`);
    },
  });

  const selectedExecution = detailQuery.data?.execution || filteredExecutions.find((item) => String(item?.id || '').trim() === selectedExecutionId) || null;
  const selectedWorkers = Array.isArray(detailQuery.data?.workers) ? detailQuery.data.workers : [];
  const selectedTerminal = isExecutionTerminalStatus(selectedExecution?.status);
  const selectedHasFailedTasks = executionHasFailedTasks(selectedExecution);
  const selectedPolicy = selectedExecution?.policy && typeof selectedExecution.policy === 'object' ? selectedExecution.policy : {};
  const selectedPolicyAskPending =
    !!selectedExecution &&
    !selectedTerminal &&
    String(selectedPolicy?.decision || '').trim() === 'ask' &&
    !String(selectedPolicy?.approvedAt || '').trim();

  const setParam = (key: string, value: string) => {
    const next = new URLSearchParams(searchParams);
    if (!value || value === 'all') next.delete(key);
    else next.set(key, value);
    setSearchParams(next, { replace: true });
  };

  return {
    featureFlags,
    authz,
    searchValue,
    statusFilter,
    templateFilter,
    triggerFilter,
    executionsQuery,
    detailQuery,
    executions,
    filteredExecutions,
    templateOptions,
    triggerOptions,
    selectedExecutionId,
    setSelectedExecutionId,
    selectedExecution,
    selectedWorkers,
    selectedTerminal,
    selectedHasFailedTasks,
    selectedPolicyAskPending,
    cancelMutation,
    approveMutation,
    derivedMutation,
    refreshExecutions: () => queryClient.invalidateQueries({ queryKey: ['executions'] }),
    setParam,
    navigate,
    executionCounts,
  };
}
