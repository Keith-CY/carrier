import { formatAgeSeconds, formatDateTime } from '../../lib/format';
import { useWorkersData } from './useWorkersData';

function workerStateBadgeClass(state: unknown): string {
  const normalized = String(state || '').trim().toLowerCase();
  if (normalized === 'available' || normalized === 'managed') return 'badge badge-ok';
  if (normalized === 'busy' || normalized === 'provisioning' || normalized === 'reclaiming' || normalized === 'ready') return 'badge badge-warn';
  if (normalized === 'error') return 'badge badge-error';
  return 'badge badge-unknown';
}

export function WorkersPage() {
  const {
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
    refreshWorkers,
    reclaimIdle,
    reclaimStale,
  } = useWorkersData();

  const queueSummary = summaryPayload.queueSummary || {};

  return (
    <section id="view-workers" className="view">
      <div className="section-head">
        <h2>Workers</h2>
        <div className="section-actions">
          <button id="workers-reclaim-stale" className="btn-sm" type="button" onClick={reclaimStale}>
            Reclaim Stale
          </button>
          <button id="workers-reclaim-idle" className="btn-sm btn-secondary" type="button" onClick={reclaimIdle}>
            Reclaim Idle
          </button>
          <button id="workers-refresh" className="btn-sm btn-secondary" type="button" onClick={() => void refreshWorkers()}>
            Refresh
          </button>
        </div>
      </div>
      <div className="card executions-toolbar">
        <div className="form-grid">
          <div>
            <label htmlFor="workers-search">Search</label>
            <input id="workers-search" type="text" placeholder="Search by host, agent, source, or execution" value={searchValue} onChange={(event) => setSearchValue(event.target.value)} />
          </div>
          <div>
            <label htmlFor="workers-state-filter">State</label>
            <select id="workers-state-filter" value={stateFilter} onChange={(event) => setStateFilter(event.target.value)}>
              <option value="all">All</option>
              <option value="active">Active</option>
              <option value="available">Available</option>
              <option value="managed">Managed</option>
              <option value="stopped">Stopped</option>
              <option value="error">Error</option>
              <option value="stale">Stale</option>
              <option value="reclaimed">Reclaimed</option>
            </select>
          </div>
        </div>
        <p id="workers-summary" className="text-dim">
          {summaryPayload.total
            ? `Total: ${summaryPayload.total} · Visible: ${filteredWorkers.length} · Active: ${summaryPayload.active} · Busy: ${summaryPayload.busy} · Stale: ${summaryPayload.stale} · Active Executions: ${Number(queueSummary.activeExecutions || 0) || 0} · Queued Tasks: ${Number(queueSummary.queuedTasks || 0) || 0} · Local: ${summaryPayload.local} · Remote: ${summaryPayload.remote}`
            : 'No workers discovered yet.'}
        </p>
        <div id="workers-msg">
          {workersQuery.data?.warnings?.length ? <p className="msg-error">{workersQuery.data.warnings.join(' | ')}</p> : null}
          {message.text ? <p className={`msg-${message.type}`}>{message.text}</p> : null}
        </div>
      </div>
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
    </section>
  );
}
