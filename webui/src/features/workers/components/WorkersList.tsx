import { formatAgeSeconds, formatDateTime } from '../../../lib/format';
import type { WorkersData } from '../useWorkersData';

function workerStateBadgeClass(state: unknown): string {
  const normalized = String(state || '').trim().toLowerCase();
  if (normalized === 'available' || normalized === 'managed') return 'badge badge-ok';
  if (normalized === 'busy' || normalized === 'provisioning' || normalized === 'reclaiming' || normalized === 'ready') return 'badge badge-warn';
  if (normalized === 'error') return 'badge badge-error';
  return 'badge badge-unknown';
}

export function WorkersList({ data }: { data: WorkersData }) {
  const { filteredWorkers, workerMetaLines } = data;

  return (
    <div id="workers-list" className="executions-list">
      {filteredWorkers.length ? filteredWorkers.map((worker: any) => (
        <div key={String(worker?.id || Math.random())} className="agent-card worker-card">
          <div className="section-head">
            <div>
              <h4>{String(worker?.hostName || worker?.hostId || 'unknown')} / {String(worker?.agentId || 'unknown')}</h4>
              <div className="instance-meta">source: {String(worker?.source || 'unknown')} · id: {String(worker?.id || 'n/a')}</div>
            </div>
            <div className="worker-badge-row">
              <span className={workerStateBadgeClass(worker?.state)}>{String(worker?.state || 'unknown')}</span>
              {worker?.stale ? <span className="badge badge-warn">stale</span> : null}
            </div>
          </div>
          <div className="worker-meta-grid">
            {workerMetaLines(worker).map((line) => <div key={line} className="execution-detail-line">{line}</div>)}
          </div>
          {String(worker?.lastError || '').trim() ? <pre className="code-block execution-result-output">{String(worker.lastError).trim()}</pre> : null}
        </div>
      )) : <div className="card">No workers match the current filter.</div>}
    </div>
  );
}
