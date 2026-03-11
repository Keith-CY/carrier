import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiGet, apiPost } from '../../lib/api';
import { useFeatures } from '../../app/useFeatures';
import { buildWorkerSummaryPayload, filterWorkers, normalizeWorkers, workerMetaLines } from './model';

export function useWorkersData() {
  const queryClient = useQueryClient();
  const { featureFlags } = useFeatures();
  const [searchValue, setSearchValue] = useState('');
  const [stateFilter, setStateFilter] = useState('all');
  const [message, setMessage] = useState<{ type: string; text: string }>({ type: 'info', text: '' });

  const workersQuery = useQuery({
    queryKey: ['workers'],
    queryFn: async () => {
      const [payload, queuePayload] = await Promise.all([
        apiGet<any>('/api/v1/orchestrator/workers'),
        apiGet<any>('/api/v1/orchestrator/workers/queue'),
      ]);
      return {
        workers: normalizeWorkers(payload),
        summary: payload?.summary || {},
        warnings: Array.isArray(payload?.warnings) ? payload.warnings : [],
        queue: queuePayload?.summary || {},
      };
    },
    enabled: featureFlags.remoteControlPlaneEnabled,
    refetchInterval: featureFlags.remoteControlPlaneEnabled ? 5000 : false,
  });

  const reclaimMutation = useMutation({
    mutationFn: async (staleOnly: boolean) => {
      const path = staleOnly ? '/api/v1/orchestrator/workers/reclaim-stale' : '/api/v1/orchestrator/workers/reclaim';
      const payload = await apiPost<any>(path, {});
      return { staleOnly, reclaim: payload?.reclaim || {} };
    },
    onSuccess: async ({ staleOnly, reclaim }) => {
      await queryClient.invalidateQueries({ queryKey: ['workers'] });
      setMessage({
        type: 'info',
        text: `${staleOnly ? 'Stale reclaim' : 'Idle reclaim'} finished: reclaimed=${Number(reclaim?.reclaimed || 0) || 0}, skipped=${Number(reclaim?.skipped || 0) || 0}, failed=${Number(reclaim?.failed || 0) || 0}`,
      });
    },
    onError: (error: Error, staleOnly) => {
      setMessage({ type: 'error', text: `${staleOnly ? 'Stale reclaim' : 'Reclaim'} failed: ${error.message}` });
    },
  });

  const workers = workersQuery.data?.workers || [];
  const summaryPayload = buildWorkerSummaryPayload(workers, workersQuery.data?.queue || {});
  const filteredWorkers = useMemo(() => filterWorkers(workers, searchValue, stateFilter), [workers, searchValue, stateFilter]);

  return {
    featureFlags,
    searchValue,
    setSearchValue,
    stateFilter,
    setStateFilter,
    message,
    workersQuery,
    summaryPayload,
    filteredWorkers,
    workerMetaLines,
    refreshWorkers: () => queryClient.invalidateQueries({ queryKey: ['workers'] }),
    reclaimIdle: () => reclaimMutation.mutate(false),
    reclaimStale: () => reclaimMutation.mutate(true),
  };
}
