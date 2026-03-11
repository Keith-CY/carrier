import type { WorkersData } from './useWorkersData';
import { WorkersList } from './components/WorkersList';
import { WorkersToolbarCard } from './components/WorkersToolbarCard';

export function WorkersSection({ data }: { data: WorkersData }) {
  return (
    <section id="view-workers" className="view">
      <div className="section-head">
        <h2>Workers</h2>
        <div className="section-actions">
          <button id="workers-reclaim-stale" className="btn-sm" type="button" onClick={data.reclaimStale}>
            Reclaim Stale
          </button>
          <button id="workers-reclaim-idle" className="btn-sm btn-secondary" type="button" onClick={data.reclaimIdle}>
            Reclaim Idle
          </button>
          <button id="workers-refresh" className="btn-sm btn-secondary" type="button" onClick={() => void data.refreshWorkers()}>
            Refresh
          </button>
        </div>
      </div>
      <WorkersToolbarCard data={data} />
      <WorkersList data={data} />
    </section>
  );
}
