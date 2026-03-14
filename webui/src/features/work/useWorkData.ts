import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';
import { useFeatures } from '../../app/useFeatures';
import { apiGet, apiPost } from '../../lib/api';
import {
  normalizeWorkItem,
  normalizeWorkItems,
  normalizeWorkProject,
  normalizeWorkProjects,
  normalizeWorkRun,
  normalizeWorkRuns,
} from './model';

function useWorkAccess() {
  const queryClient = useQueryClient();
  const { featureFlags, authz } = useFeatures();
  const enabled = featureFlags.remoteControlPlaneEnabled && authz.permissions.viewExecutions;

  return {
    featureFlags,
    authz,
    enabled,
    refresh: () => queryClient.invalidateQueries({ queryKey: ['work'] }),
  };
}

export function useWorkPageData() {
  const access = useWorkAccess();
  const query = useQuery({
    queryKey: ['work', 'landing'],
    queryFn: async () => {
      const [projectsPayload, itemsPayload, runsPayload] = await Promise.all([
        apiGet<any>('/api/v1/work/projects'),
        apiGet<any>('/api/v1/work/items'),
        apiGet<any>('/api/v1/work/runs'),
      ]);
      return {
        projects: normalizeWorkProjects(projectsPayload),
        items: normalizeWorkItems(itemsPayload),
        runs: normalizeWorkRuns(runsPayload),
      };
    },
    enabled: access.enabled,
  });

  return { ...access, query };
}

export function useWorkProjectPageData() {
  const access = useWorkAccess();
  const { projectId = '' } = useParams();
  const query = useQuery({
    queryKey: ['work', 'project-page', projectId],
    queryFn: async () => {
      const [projectPayload, itemsPayload, runsPayload] = await Promise.all([
        apiGet<any>(`/api/v1/work/projects/${encodeURIComponent(projectId)}`),
        apiGet<any>('/api/v1/work/items'),
        apiGet<any>('/api/v1/work/runs'),
      ]);
      const project = normalizeWorkProject(projectPayload);
      if (!project) throw new Error('Work project not found');
      const items = normalizeWorkItems(itemsPayload).filter((item) => item.projectId === project.id);
      const runs = normalizeWorkRuns(runsPayload).filter((run) => run.projectId === project.id);
      return { project, items, runs };
    },
    enabled: access.enabled && !!projectId,
  });

  const syncMutation = useMutation({
    mutationFn: () => apiPost<any>(`/api/v1/work/projects/${encodeURIComponent(projectId)}/sync`, {}),
    onSuccess: () => access.refresh(),
  });

  const createItemMutation = useMutation({
    mutationFn: (payload: { title: string; description: string; priority: string }) =>
      apiPost<any>('/api/v1/work/items', {
        projectId,
        title: payload.title,
        description: payload.description,
        priority: payload.priority,
      }),
    onSuccess: () => access.refresh(),
  });

  const importMutation = useMutation({
    mutationFn: (payload: { repository: string; issueNumber?: number; pullRequestNumber?: number }) =>
      apiPost<any>('/api/v1/work/adapters/github/import', {
        projectId,
        repository: payload.repository,
        issueNumber: payload.issueNumber,
        pullRequestNumber: payload.pullRequestNumber,
      }),
    onSuccess: () => access.refresh(),
  });

  return { ...access, projectId, query, syncMutation, createItemMutation, importMutation };
}

export function useWorkItemPageData() {
  const access = useWorkAccess();
  const { itemId = '' } = useParams();
  const query = useQuery({
    queryKey: ['work', 'item-page', itemId],
    queryFn: async () => {
      const itemPayload = await apiGet<any>(`/api/v1/work/items/${encodeURIComponent(itemId)}`);
      const item = normalizeWorkItem(itemPayload);
      if (!item) throw new Error('Work item not found');
      const [projectPayload, runPayload] = await Promise.all([
        item.projectId ? apiGet<any>(`/api/v1/work/projects/${encodeURIComponent(item.projectId)}`) : Promise.resolve(null),
        item.latestRunId ? apiGet<any>(`/api/v1/work/runs/${encodeURIComponent(item.latestRunId)}`) : Promise.resolve(null),
      ]);
      return {
        item,
        project: normalizeWorkProject(projectPayload),
        latestRun: normalizeWorkRun(runPayload),
      };
    },
    enabled: access.enabled && !!itemId,
  });

  const startRunMutation = useMutation({
    mutationFn: (backend: string) => apiPost<any>(`/api/v1/work/items/${encodeURIComponent(itemId)}/runs`, { backend }),
    onSuccess: () => access.refresh(),
  });

  const cancelMutation = useMutation({
    mutationFn: () => apiPost<any>(`/api/v1/work/items/${encodeURIComponent(itemId)}/cancel`, {}),
    onSuccess: () => access.refresh(),
  });

  const completeMutation = useMutation({
    mutationFn: () => apiPost<any>(`/api/v1/work/items/${encodeURIComponent(itemId)}/complete`, {}),
    onSuccess: () => access.refresh(),
  });

  const resumeLatestRunMutation = useMutation({
    mutationFn: () => {
      const latestRunID = String(query.data?.latestRun?.id || '').trim();
      if (!latestRunID) {
        throw new Error('No latest run is available');
      }
      return apiPost<any>(`/api/v1/work/runs/${encodeURIComponent(latestRunID)}/resume`, {});
    },
    onSuccess: () => access.refresh(),
  });

  return { ...access, itemId, query, startRunMutation, cancelMutation, completeMutation, resumeLatestRunMutation };
}

export function useWorkRunPageData() {
  const access = useWorkAccess();
  const { runId = '' } = useParams();
  const query = useQuery({
    queryKey: ['work', 'run-page', runId],
    queryFn: async () => {
      const runPayload = await apiGet<any>(`/api/v1/work/runs/${encodeURIComponent(runId)}`);
      const run = normalizeWorkRun(runPayload);
      if (!run) throw new Error('Work run not found');
      const itemPayload = run.workItemId ? await apiGet<any>(`/api/v1/work/items/${encodeURIComponent(run.workItemId)}`) : null;
      const item = normalizeWorkItem(itemPayload);
      const projectPayload = run.projectId ? await apiGet<any>(`/api/v1/work/projects/${encodeURIComponent(run.projectId)}`) : null;
      return {
        run,
        item,
        project: normalizeWorkProject(projectPayload),
      };
    },
    enabled: access.enabled && !!runId,
  });

  const resumeMutation = useMutation({
    mutationFn: () => apiPost<any>(`/api/v1/work/runs/${encodeURIComponent(runId)}/resume`, {}),
    onSuccess: () => access.refresh(),
  });

  const cancelMutation = useMutation({
    mutationFn: () => apiPost<any>(`/api/v1/work/runs/${encodeURIComponent(runId)}/cancel`, {}),
    onSuccess: () => access.refresh(),
  });

  const reclaimMutation = useMutation({
    mutationFn: () => apiPost<any>(`/api/v1/work/runs/${encodeURIComponent(runId)}/reclaim`, {}),
    onSuccess: () => access.refresh(),
  });

  const cleanupMutation = useMutation({
    mutationFn: () => apiPost<any>(`/api/v1/work/runs/${encodeURIComponent(runId)}/cleanup`, {}),
    onSuccess: () => access.refresh(),
  });

  return { ...access, runId, query, resumeMutation, cancelMutation, reclaimMutation, cleanupMutation };
}
