import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiGet, apiPost } from '../../lib/api';
import { useFeatures } from '../../app/useFeatures';
import { buildMemoryInstanceAction, buildMemorySearchPayload, normalizeMemoryPayload } from './model';

export function useMemoryData() {
  const queryClient = useQueryClient();
  const { authz } = useFeatures();
  const [subject, setSubject] = useState('agent-a');
  const [searchQuery, setSearchQuery] = useState('');
  const [searchLimit, setSearchLimit] = useState('10');
  const [searchMinScore, setSearchMinScore] = useState('0');
  const [instanceId, setInstanceId] = useState('');
  const [instanceScope, setInstanceScope] = useState('');
  const [distillReason, setDistillReason] = useState('');
  const [distillDryRun, setDistillDryRun] = useState(false);
  const [message, setMessage] = useState<{ type: string; text: string }>({ type: 'info', text: '' });
  const [actionMessage, setActionMessage] = useState<{ type: string; text: string }>({ type: 'info', text: '' });
  const [searchResults, setSearchResults] = useState<any[]>([]);

  const canMutate = authz.permissions.launchExecutions;
  const readOnly = !authz.permissions.viewExecutions;

  const memoryQuery = useQuery({
    queryKey: ['memory', subject],
    queryFn: () => apiGet<any>(subject.trim() ? `/api/v1/memory?subject=${encodeURIComponent(subject.trim())}` : '/api/v1/memory'),
  });

  const searchMutation = useMutation({
    mutationFn: (payload: any) => apiPost<any>('/api/v1/memory/search', payload),
    onSuccess: (payload) => {
      setSearchResults(Array.isArray(payload?.results) ? payload.results : []);
      setMessage({ type: 'success', text: 'Search completed.' });
    },
    onError: (error: Error) => {
      setSearchResults([]);
      setMessage({ type: 'error', text: `Search failed: ${error.message}` });
    },
  });

  const actionMutation = useMutation({
    mutationFn: async (action: 'attach' | 'detach' | 'distill') => {
      const result = buildMemoryInstanceAction({
        action,
        instanceId,
        scope: instanceScope,
        reason: distillReason,
        dryRun: distillDryRun,
      });
      if ('error' in result) throw new Error(result.error);
      return { action, result: await apiPost<any>(result.path, result.payload) };
    },
    onSuccess: async ({ action, result }) => {
      if (action === 'distill') {
        const run = result?.result && typeof result.result === 'object' ? result.result : {};
        setActionMessage({
          type: 'success',
          text: `distill ${String(run?.runId || 'unknown')} · ${String(run?.instanceId || instanceId.trim())} · ${String(run?.status || 'unknown')}`,
        });
      } else {
        setActionMessage({ type: 'success', text: String(result?.status || action) });
      }
      await queryClient.invalidateQueries({ queryKey: ['memory', subject] });
    },
    onError: (error: Error) => {
      setActionMessage({ type: 'error', text: `Memory action failed: ${error.message}` });
    },
  });

  const payload = normalizeMemoryPayload(memoryQuery.data, subject);

  return {
    authz,
    canMutate,
    readOnly,
    subject,
    setSubject,
    searchQuery,
    setSearchQuery,
    searchLimit,
    setSearchLimit,
    searchMinScore,
    setSearchMinScore,
    instanceId,
    setInstanceId,
    instanceScope,
    setInstanceScope,
    distillReason,
    setDistillReason,
    distillDryRun,
    setDistillDryRun,
    message,
    actionMessage,
    searchResults,
    memoryQuery,
    payload,
    refreshMemory: () => queryClient.invalidateQueries({ queryKey: ['memory', subject] }),
    runSearch: () => {
      const result = buildMemorySearchPayload({
        subject,
        query: searchQuery,
        limit: searchLimit,
        minScore: searchMinScore,
      });
      if ('error' in result) {
        setMessage({ type: 'error', text: result.error });
        return;
      }
      searchMutation.mutate(result.payload);
    },
    runAction: (action: 'attach' | 'detach' | 'distill') => actionMutation.mutate(action),
  };
}

export type MemoryData = ReturnType<typeof useMemoryData>;
