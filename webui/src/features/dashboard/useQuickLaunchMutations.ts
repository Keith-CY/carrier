import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { apiPost } from '../../lib/api';
import { buildQuickLaunchPreviewRequest, type QuickLaunchDraft } from './model';

export function useQuickLaunchMutations(args: {
  quickLaunchDraft: QuickLaunchDraft;
  quickLaunchPlan: any | null;
  setQuickLaunchPlan: (value: any | null) => void;
  setQuickLaunchMessage: (value: { type: string; text: string }) => void;
}) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const previewMutation = useMutation({
    mutationFn: (payload: any) => apiPost<any>('/api/v1/orchestrator/plans', payload),
    onSuccess: (data) => {
      args.setQuickLaunchPlan(data?.plan || {});
      args.setQuickLaunchMessage({ type: 'info', text: 'Preview ready. Confirm to create and authorize the execution.' });
    },
    onError: (error: Error) => {
      args.setQuickLaunchPlan(null);
      args.setQuickLaunchMessage({ type: 'error', text: `Preview failed: ${error.message}` });
    },
  });

  const runMutation = useMutation({
    mutationFn: async () => {
      if (!args.quickLaunchPlan) throw new Error('Preview a plan before running.');
      const created = await apiPost<any>('/api/v1/orchestrator/executions', {
        goal: String(args.quickLaunchPlan.goal || '').trim(),
        templateId: String(args.quickLaunchPlan.templateId || '').trim(),
        templateVersion: String(args.quickLaunchPlan.templateVersion || '').trim(),
        requestedProvider: String(args.quickLaunchPlan.provider || '').trim(),
        approvalScope: String(args.quickLaunchPlan.approvalScope || 'infrastructure_only').trim(),
        requiredMemory: Array.isArray(args.quickLaunchPlan.requiredMemory) ? args.quickLaunchPlan.requiredMemory : [],
        distillOutputs: Array.isArray(args.quickLaunchPlan.distillOutputs) ? args.quickLaunchPlan.distillOutputs : [],
        requiredWorkers: Array.isArray(args.quickLaunchPlan.requiredWorkers) ? args.quickLaunchPlan.requiredWorkers : [],
        taskUnits: Array.isArray(args.quickLaunchPlan.taskUnits) ? args.quickLaunchPlan.taskUnits : [],
        maxConcurrency: Number(args.quickLaunchPlan.maxConcurrency || 0) || 0,
      });
      const executionID = String(created?.execution?.id || '').trim();
      if (!executionID) throw new Error('create response missing execution id');
      await apiPost(`/api/v1/orchestrator/executions/${encodeURIComponent(executionID)}/authorize`, {
        approved: true,
        actor: 'webui',
        maxConcurrency: Number(args.quickLaunchPlan.maxConcurrency || 0) || 0,
      });
      return executionID;
    },
    onSuccess: async (executionId) => {
      args.setQuickLaunchMessage({ type: 'info', text: `Execution created: ${executionId}` });
      await queryClient.invalidateQueries({ queryKey: ['executions'] });
      navigate(`/executions/${encodeURIComponent(executionId)}`);
    },
    onError: (error: any) => {
      const blockedExecutionId = String(error?.payload?.execution?.id || '').trim();
      if (Number(error?.status || 0) === 409 && blockedExecutionId) {
        args.setQuickLaunchMessage({ type: 'error', text: `Execution created but waiting for policy approval: ${blockedExecutionId}` });
        navigate(`/executions/${encodeURIComponent(blockedExecutionId)}`);
        return;
      }
      args.setQuickLaunchMessage({ type: 'error', text: `Run failed: ${error.message}` });
    },
  });

  return {
    navigate,
    previewMutation,
    runMutation,
    previewQuickLaunch: () => {
      const result = buildQuickLaunchPreviewRequest(args.quickLaunchDraft);
      if ('error' in result) {
        args.setQuickLaunchMessage({ type: 'error', text: result.error });
        return;
      }
      previewMutation.mutate(result.payload);
    },
    runQuickLaunch: () => runMutation.mutate(),
  };
}
