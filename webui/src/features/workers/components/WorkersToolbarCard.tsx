import type { WorkersData } from '../useWorkersData';

export function WorkersToolbarCard({ data }: { data: WorkersData }) {
  const {
    searchValue,
    setSearchValue,
    stateFilter,
    setStateFilter,
    message,
    workersQuery,
    summaryPayload,
    filteredWorkers,
  } = data;

  const queueSummary = summaryPayload.queueSummary || {};

  return (
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
  );
}
