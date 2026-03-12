import { ExecutionDetailContent } from '../../executions/ExecutionDetailContent';
import { useDashboardExecutionDetail } from '../useDashboardData';

export function DashboardExecutionDetail({ executionId }: { executionId: string }) {
  const executionDetailQuery = useDashboardExecutionDetail(executionId);
  if (executionDetailQuery.isLoading) return <div className="text-dim">Loading details…</div>;
  if (executionDetailQuery.isError) return <div className="text-dim">Load failed: {(executionDetailQuery.error as Error).message}</div>;
  const payload = executionDetailQuery.data || {};
  return (
    <ExecutionDetailContent
      execution={payload.execution || {}}
      workers={Array.isArray(payload.workers) ? payload.workers : []}
      onDownloadArtifact={async () => undefined}
    />
  );
}
