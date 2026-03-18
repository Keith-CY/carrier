import { PageShell } from '../../app/page-shell';
import type { WorkersData } from './useWorkersData';
import { WorkersList } from './components/WorkersList';
import { WorkersToolbarCard } from './components/WorkersToolbarCard';

export function WorkersSection({ data }: { data: WorkersData }) {
  const filteredWorkers = Array.isArray(data.filteredWorkers) ? data.filteredWorkers : [];
  const summaryPayload = data.summaryPayload || {};

  return (
    <PageShell
      id="view-workers"
      eyebrow="Operate"
      title="Workers"
      description="Track active worker slots, reclaim stale leases, and see which host and agent combinations are actually carrying load."
      actions={(
        <>
          <button id="workers-reclaim-stale" className="btn-sm" type="button" onClick={data.reclaimStale}>
            Reclaim Stale
          </button>
          <button id="workers-reclaim-idle" className="btn-sm btn-secondary" type="button" onClick={data.reclaimIdle}>
            Reclaim Idle
          </button>
          <button id="workers-refresh" className="btn-sm btn-secondary" type="button" onClick={() => void data.refreshWorkers()}>
            Refresh
          </button>
        </>
      )}
      stats={[
        { label: 'Visible Workers', value: String(filteredWorkers.length) },
        { label: 'Total', value: String(summaryPayload.total || 0) },
        { label: 'Active', value: String(summaryPayload.active || 0) },
        { label: 'Busy', value: String(summaryPayload.busy || 0) },
        { label: 'Stale', value: String(summaryPayload.stale || 0) },
      ]}
    >
      <WorkersToolbarCard data={data} />
      <WorkersList data={data} />
    </PageShell>
  );
}
