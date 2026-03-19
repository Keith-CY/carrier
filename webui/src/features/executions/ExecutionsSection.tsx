import { PageShell } from '../../app/page-shell';
import type { ExecutionsData } from './useExecutionsData';
import { ExecutionDetailCard } from './components/ExecutionDetailCard';
import { ExecutionsListCard } from './components/ExecutionsListCard';
import { ExecutionsToolbarCard } from './components/ExecutionsToolbarCard';

export function ExecutionsSection({ data }: { data: ExecutionsData }) {
  const executions = Array.isArray(data.executions) ? data.executions : [];
  const filteredExecutions = Array.isArray(data.filteredExecutions) ? data.filteredExecutions : [];

  return (
    <PageShell
      id="view-executions"
      eyebrow="Operate"
      title="Executions"
      description="Filter active plans, inspect selected execution details, and resolve approvals or recovery actions in place."
      actions={(
        <button id="executions-refresh" className="btn-sm btn-secondary" onClick={() => void data.refreshExecutions()}>
          Refresh
        </button>
      )}
      stats={[
        { label: 'Total', value: String(executions.length) },
        { label: 'Visible', value: String(filteredExecutions.length) },
        { label: 'Selected', value: data.selectedExecutionId || 'None' },
        { label: 'Approval', value: data.selectedPolicyAskPending ? 'Pending' : String(data.selectedExecution?.status || 'Idle') },
      ]}
    >
      <ExecutionsToolbarCard data={data} />

      <div className="executions-layout">
        <ExecutionsListCard data={data} />
        <ExecutionDetailCard data={data} />
      </div>
    </PageShell>
  );
}
