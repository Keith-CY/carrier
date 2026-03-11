import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { useFeatures } from '../../app/useFeatures';
import { apiGet, apiPost } from '../../lib/api';
import { executionCounts, isExecutionTerminalStatus } from '../executions/ExecutionDetailContent';
import {
  buildQuickLaunchPreviewRequest,
  defaultQuickLaunchDraft,
  flattenProviderCatalog,
  normalizeAgentCatalog,
  normalizeExecutions,
  normalizeInstances,
  toggleHostSelection,
  type QuickLaunchDraft,
  type QuickLaunchMode,
} from './model';

export function useDashboardData() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { featureFlags, authz } = useFeatures();
  const [quickLaunchMessage, setQuickLaunchMessage] = useState<{ type: string; text: string }>({ type: 'info', text: '' });
  const [quickLaunchDraft, setQuickLaunchDraft] = useState<QuickLaunchDraft>(() => defaultQuickLaunchDraft());
  const [quickLaunchAdvancedVisible, setQuickLaunchAdvancedVisible] = useState(false);
  const [quickLaunchPlan, setQuickLaunchPlan] = useState<any | null>(null);
  const [expandedExecutionIds, setExpandedExecutionIds] = useState<Set<string>>(new Set());
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
  const executionsQuery = useQuery({
    queryKey: ['executions'],
    queryFn: () => apiGet<any>('/api/v1/orchestrator/executions'),
    enabled: featureFlags.remoteControlPlaneEnabled && authz.permissions.viewExecutions,
    refetchInterval: featureFlags.remoteControlPlaneEnabled && authz.permissions.viewExecutions ? 5000 : false,
  });
  const providersQuery = useQuery({
    queryKey: ['provider-catalog'],
    queryFn: () => apiGet<any>('/api/v1/providers'),
    enabled: featureFlags.remoteControlPlaneEnabled && authz.permissions.launchExecutions,
  });
  const templatesQuery = useQuery({
    queryKey: ['execution-templates'],
    queryFn: () => apiGet<any>('/api/v1/templates'),
    enabled: featureFlags.remoteControlPlaneEnabled && authz.permissions.launchExecutions,
  });
  const hostsQuery = useQuery({
    queryKey: ['remote-hosts'],
    queryFn: () => apiGet<any>('/api/v1/remote/hosts'),
    enabled: featureFlags.remoteControlPlaneEnabled && authz.permissions.launchExecutions,
  });

  const instances = useMemo(() => normalizeInstances(instancesQuery.data), [instancesQuery.data]);
  const agentCatalog = useMemo(() => normalizeAgentCatalog(agentCatalogQuery.data), [agentCatalogQuery.data]);
  const executions = useMemo(() => normalizeExecutions(executionsQuery.data), [executionsQuery.data]);
  const providerOptions = useMemo(() => flattenProviderCatalog(providersQuery.data), [providersQuery.data]);
  const templates = useMemo(() => Array.isArray(templatesQuery.data?.templates) ? templatesQuery.data.templates : [], [templatesQuery.data]);
  const hosts = useMemo(() => Array.isArray(hostsQuery.data?.hosts) ? hostsQuery.data.hosts : [], [hostsQuery.data]);
  const selectedTemplate = useMemo(
    () => templates.find((item: any) => String(item?.id || '').trim() === quickLaunchDraft.templateId) || null,
    [templates, quickLaunchDraft.templateId],
  );
  const hostOptions = useMemo(() => {
    const items = [{ id: 'local', name: 'local' }].concat(hosts);
    const seen = new Set<string>();
    return items.filter((item: any) => {
      const id = String(item?.id || '').trim();
      if (!id || seen.has(id)) return false;
      seen.add(id);
      return true;
    });
  }, [hosts]);

  const previewMutation = useMutation({
    mutationFn: (payload: any) => apiPost<any>('/api/v1/orchestrator/plans', payload),
    onSuccess: (data) => {
      setQuickLaunchPlan(data?.plan || {});
      setQuickLaunchMessage({ type: 'info', text: 'Preview ready. Confirm to create and authorize the execution.' });
    },
    onError: (error: Error) => {
      setQuickLaunchPlan(null);
      setQuickLaunchMessage({ type: 'error', text: `Preview failed: ${error.message}` });
    },
  });

  const runMutation = useMutation({
    mutationFn: async () => {
      if (!quickLaunchPlan) throw new Error('Preview a plan before running.');
      const created = await apiPost<any>('/api/v1/orchestrator/executions', {
        goal: String(quickLaunchPlan.goal || '').trim(),
        templateId: String(quickLaunchPlan.templateId || '').trim(),
        requestedProvider: String(quickLaunchPlan.provider || '').trim(),
        approvalScope: String(quickLaunchPlan.approvalScope || 'infrastructure_only').trim(),
        requiredWorkers: Array.isArray(quickLaunchPlan.requiredWorkers) ? quickLaunchPlan.requiredWorkers : [],
        taskUnits: Array.isArray(quickLaunchPlan.taskUnits) ? quickLaunchPlan.taskUnits : [],
        maxConcurrency: Number(quickLaunchPlan.maxConcurrency || 0) || 0,
      });
      const executionID = String(created?.execution?.id || '').trim();
      if (!executionID) throw new Error('create response missing execution id');
      await apiPost(`/api/v1/orchestrator/executions/${encodeURIComponent(executionID)}/authorize`, {
        approved: true,
        actor: 'webui',
        maxConcurrency: Number(quickLaunchPlan.maxConcurrency || 0) || 0,
      });
      return executionID;
    },
    onSuccess: async (executionId) => {
      setQuickLaunchMessage({ type: 'info', text: `Execution created: ${executionId}` });
      await queryClient.invalidateQueries({ queryKey: ['executions'] });
      navigate(`/executions/${encodeURIComponent(executionId)}`);
    },
    onError: (error: any) => {
      const blockedExecutionId = String(error?.payload?.execution?.id || '').trim();
      if (Number(error?.status || 0) === 409 && blockedExecutionId) {
        setQuickLaunchMessage({ type: 'error', text: `Execution created but waiting for policy approval: ${blockedExecutionId}` });
        navigate(`/executions/${encodeURIComponent(blockedExecutionId)}`);
        return;
      }
      setQuickLaunchMessage({ type: 'error', text: `Run failed: ${error.message}` });
    },
  });

  const refreshExecutions = async () => {
    await queryClient.invalidateQueries({ queryKey: ['executions'] });
  };

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

  const recentExecutions = executions.slice(0, 8);
  const activeRecentExecutions = recentExecutions.filter((item) => !isExecutionTerminalStatus(item?.status)).length;

  return {
    featureFlags,
    authz,
    instancesQuery,
    agentCatalogQuery,
    executionsQuery,
    templatesQuery,
    addAgentModalOpen,
    setAddAgentModalOpen,
    agentCatalog,
    instances,
    runningInstances,
    recentExecutions,
    activeRecentExecutions,
    expandedExecutionIds,
    quickLaunchMessage,
    quickLaunchDraft,
    quickLaunchAdvancedVisible,
    quickLaunchPlan,
    previewMutation,
    runMutation,
    providerOptions,
    templates,
    selectedTemplate,
    hostOptions,
    setQuickLaunchMode: (mode: QuickLaunchMode) => setQuickLaunchDraft((current) => ({ ...current, mode })),
    setQuickLaunchGoal: (goal: string) => setQuickLaunchDraft((current) => ({ ...current, goal })),
    setQuickLaunchProvider: (provider: string) => setQuickLaunchDraft((current) => ({ ...current, provider })),
    setQuickLaunchMaxConcurrency: (maxConcurrency: string) => setQuickLaunchDraft((current) => ({ ...current, maxConcurrency })),
    setQuickLaunchHostLabels: (hostLabels: string) => setQuickLaunchDraft((current) => ({ ...current, hostLabels })),
    setQuickLaunchTemplateId: (templateId: string) => setQuickLaunchDraft((current) => ({ ...current, templateId, templateInputs: {} })),
    setQuickLaunchTemplateInput: (key: string, value: string) => setQuickLaunchDraft((current) => ({
      ...current,
      templateInputs: { ...current.templateInputs, [key]: value },
    })),
    toggleQuickLaunchHost: (hostId: string) => setQuickLaunchDraft((current) => ({
      ...current,
      selectedHosts: toggleHostSelection(current.selectedHosts, hostId),
    })),
    resetQuickLaunch: () => {
      setQuickLaunchDraft(defaultQuickLaunchDraft());
      setQuickLaunchPlan(null);
      setQuickLaunchMessage({ type: 'info', text: '' });
    },
    clearQuickLaunchPreview: () => setQuickLaunchPlan(null),
    setQuickLaunchAdvancedVisible,
    previewQuickLaunch: () => {
      const result = buildQuickLaunchPreviewRequest(quickLaunchDraft);
      if ('error' in result) {
        setQuickLaunchMessage({ type: 'error', text: result.error });
        return;
      }
      previewMutation.mutate(result.payload);
    },
    runQuickLaunch: () => runMutation.mutate(),
    refreshExecutions,
    refreshInstances,
    handleInstanceAction,
    toggleExecutionExpansion: (executionId: string) => setExpandedExecutionIds((current) => {
      const next = new Set(current);
      if (next.has(executionId)) next.delete(executionId);
      else next.add(executionId);
      return next;
    }),
    executionCounts,
    navigate,
  };
}

export function useDashboardExecutionDetail(executionId: string) {
  return useQuery({
    queryKey: ['execution-detail', executionId],
    queryFn: () => apiGet<any>(`/api/v1/orchestrator/executions/${encodeURIComponent(executionId)}`),
    enabled: !!executionId,
  });
}
