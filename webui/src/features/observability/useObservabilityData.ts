import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { useFeatures } from '../../app/useFeatures';
import { apiGet } from '../../lib/api';
import { toFiniteNumber } from '../../lib/format';
import { buildObservabilityDrillDown, operationGroup, operationIsAnomalous, statusLineMetrics, topAttributionLabel } from './model';

export function useObservabilityData() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { featureFlags, authz, isLoading: featuresLoading } = useFeatures();
  const [group, setGroup] = useState('all');
  const [anomaliesOnly, setAnomaliesOnly] = useState(false);

  const remoteMetricsQuery = useQuery({
    queryKey: ['remote-observability-metrics'],
    queryFn: () => apiGet<any>('/api/v1/remote/metrics'),
    enabled: featureFlags.remoteControlPlaneEnabled && authz.permissions.viewExecutions,
    refetchInterval: featureFlags.remoteControlPlaneEnabled && authz.permissions.viewExecutions ? 5000 : false,
  });

  const orchestratorMetricsQuery = useQuery({
    queryKey: ['orchestrator-observability-metrics'],
    queryFn: () => apiGet<any>('/api/v1/orchestrator/metrics'),
    enabled: featureFlags.remoteControlPlaneEnabled && authz.permissions.viewExecutions,
    refetchInterval: featureFlags.remoteControlPlaneEnabled && authz.permissions.viewExecutions ? 5000 : false,
  });

  const refreshMutation = useMutation({
    mutationFn: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['remote-observability-metrics'] }),
        queryClient.invalidateQueries({ queryKey: ['orchestrator-observability-metrics'] }),
      ]);
    },
  });

  const remoteMetrics = remoteMetricsQuery.data?.metrics || {};
  const orchestratorMetrics = orchestratorMetricsQuery.data?.metrics || {};
  const operations = remoteMetrics?.operations && typeof remoteMetrics.operations === 'object' ? remoteMetrics.operations : {};

  const operationGroups = useMemo(() => {
    const values = Array.from(
      new Set(Object.keys(operations).map((name) => operationGroup(name)).filter(Boolean)),
    ).sort((left, right) => left.localeCompare(right));
    return ['all', ...values];
  }, [operations]);

  const visibleOperations = useMemo(() => {
    return Object.entries(operations)
      .map(([name, value]) => ({ name, metrics: value as Record<string, any> }))
      .filter((entry) => group === 'all' || operationGroup(entry.name) === group)
      .filter((entry) => !anomaliesOnly || operationIsAnomalous(entry.metrics))
      .sort((left, right) => {
        const failureDelta = toFiniteNumber(right.metrics?.failure) - toFiniteNumber(left.metrics?.failure);
        if (failureDelta !== 0) return failureDelta;
        const successRateDelta = toFiniteNumber(left.metrics?.successRate) - toFiniteNumber(right.metrics?.successRate);
        if (successRateDelta !== 0) return successRateDelta;
        const latencyDelta = toFiniteNumber(right.metrics?.avgLatencyMs) - toFiniteNumber(left.metrics?.avgLatencyMs);
        if (latencyDelta !== 0) return latencyDelta;
        return left.name.localeCompare(right.name);
      });
  }, [operations, group, anomaliesOnly]);

  const providerMetrics = orchestratorMetrics?.providers && typeof orchestratorMetrics.providers === 'object'
    ? orchestratorMetrics.providers
    : {};
  const policyMetrics = orchestratorMetrics?.policies && typeof orchestratorMetrics.policies === 'object'
    ? orchestratorMetrics.policies
    : {};

  const topTeam = topAttributionLabel(providerMetrics?.attribution?.teams || []);
  const topProject = topAttributionLabel(providerMetrics?.attribution?.projects || []);
  const topTemplate = topAttributionLabel(providerMetrics?.attribution?.templates || []);
  const topTrigger = topAttributionLabel(providerMetrics?.attribution?.triggers || []);
  const topProvider = String((providerMetrics?.aggregates || []).slice().sort((left: any, right: any) => {
    const costDelta = toFiniteNumber(right?.estimatedCostUsd) - toFiniteNumber(left?.estimatedCostUsd);
    if (costDelta !== 0) return costDelta;
    return String(left?.provider || '').localeCompare(String(right?.provider || ''));
  })[0]?.provider || 'none').trim() || 'none';
  const topModelEntry = (providerMetrics?.models || []).slice().sort((left: any, right: any) => {
    const costDelta = toFiniteNumber(right?.estimatedCostUsd) - toFiniteNumber(left?.estimatedCostUsd);
    if (costDelta !== 0) return costDelta;
    return String(left?.model || '').localeCompare(String(right?.model || ''));
  })[0];
  const topModel = String(topModelEntry?.model || 'none').trim() || 'none';

  useEffect(() => {
    if (featuresLoading) return;
    if (!featureFlags.remoteControlPlaneEnabled || !authz.permissions.viewExecutions) {
      navigate('/dashboard', { replace: true });
    }
  }, [authz.permissions.viewExecutions, featureFlags.remoteControlPlaneEnabled, featuresLoading, navigate]);

  return {
    featureFlags,
    authz,
    featuresLoading,
    group,
    setGroup,
    anomaliesOnly,
    setAnomaliesOnly,
    remoteMetricsQuery,
    orchestratorMetricsQuery,
    refreshMutation,
    remoteMetrics,
    orchestratorMetrics,
    providerMetrics,
    policyMetrics,
    operationGroups,
    visibleOperations,
    topTeam,
    topProject,
    topTemplate,
    topTrigger,
    topProvider,
    topModel,
    statusLine: statusLineMetrics(remoteMetrics, orchestratorMetrics, anomaliesOnly, group),
    drillDown: (type: 'team' | 'project' | 'template' | 'trigger', value: string) => navigate(buildObservabilityDrillDown(type, value)),
    drillDownHref: buildObservabilityDrillDown,
  };
}
