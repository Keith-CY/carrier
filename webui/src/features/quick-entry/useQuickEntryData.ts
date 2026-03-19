import { useMemo } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { usePWAInstall } from '../../app/pwa';
import { useFeatures } from '../../app/useFeatures';
import { apiGet, apiPost } from '../../lib/api';
import { normalizeExecutions } from '../dashboard/model';
import { useChatData } from '../chat/useChatData';

const TERMINAL_STATUSES = new Set(['completed', 'partial_completed', 'failed', 'retryable_failed', 'declined', 'cancelled']);

function executionStatus(value: unknown) {
  return String(value || '').trim().toLowerCase();
}

function isPendingApproval(execution: any) {
  const status = executionStatus(execution?.status);
  if (status === 'pending_authorization') return true;
  const decision = String(execution?.policy?.decision || '').trim().toLowerCase();
  const approvedAt = String(execution?.policy?.approvedAt || '').trim();
  return !TERMINAL_STATUSES.has(status) && decision === 'ask' && !approvedAt;
}

function summarizeApproval(execution: any) {
  return String(
    execution?.policy?.reason ||
    execution?.policy?.summary ||
    execution?.goal ||
    'Approval required before this run can continue.',
  ).trim();
}

function summarizeActivity(execution: any) {
  return String(
    execution?.outcome?.summary ||
    execution?.policy?.summary ||
    execution?.goal ||
    'Active work is still in progress.',
  ).trim();
}

export function useQuickEntryData() {
  const chat = useChatData({ persistKey: 'quick-entry' });
  const queryClient = useQueryClient();
  const { featureFlags, authz } = useFeatures();
  const install = usePWAInstall();

  const executionsQuery = useQuery({
    queryKey: ['quick-entry', 'executions'],
    queryFn: () => apiGet<any>('/api/v1/orchestrator/executions'),
    enabled: featureFlags.remoteControlPlaneEnabled && authz.permissions.viewExecutions,
    retry: false,
    refetchInterval: featureFlags.remoteControlPlaneEnabled && authz.permissions.viewExecutions ? 5000 : false,
  });

  const executions = useMemo(() => normalizeExecutions(executionsQuery.data), [executionsQuery.data]);
  const approvals = useMemo(() => executions.filter(isPendingApproval).slice(0, 4), [executions]);
  const activeExecutions = useMemo(
    () => executions.filter((item) => !isPendingApproval(item) && !TERMINAL_STATUSES.has(executionStatus(item?.status))).slice(0, 4),
    [executions],
  );

  const invalidate = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['quick-entry', 'executions'] }),
      queryClient.invalidateQueries({ queryKey: ['home', 'executions'] }),
    ]);
  };

  const approveMutation = useMutation({
    mutationFn: (executionId: string) => apiPost(`/api/v1/orchestrator/executions/${encodeURIComponent(executionId)}/authorize`, {
      approved: true,
      actor: 'webui',
      policyApproved: true,
    }),
    onSuccess: invalidate,
  });

  const cancelMutation = useMutation({
    mutationFn: (executionId: string) => apiPost(`/api/v1/orchestrator/executions/${encodeURIComponent(executionId)}/cancel`, {
      actor: 'webui',
    }),
    onSuccess: invalidate,
  });

  const latestMessage = [...chat.messages].reverse().find((message) => message.role !== 'system') || chat.messages[chat.messages.length - 1] || null;

  return {
    chat,
    approvals,
    activeExecutions,
    approvalCount: approvals.length,
    activeCount: activeExecutions.length,
    latestMessage,
    summarizeApproval,
    summarizeActivity,
    approveExecution: (executionId: string) => approveMutation.mutateAsync(executionId),
    cancelExecution: (executionId: string) => cancelMutation.mutateAsync(executionId),
    approveMutation,
    cancelMutation,
    install,
  };
}

export type QuickEntryData = ReturnType<typeof useQuickEntryData>;
