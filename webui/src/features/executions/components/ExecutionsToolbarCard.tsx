import type { ExecutionsData } from '../useExecutionsData';

export function ExecutionsToolbarCard({ data }: { data: ExecutionsData }) {
  const {
    featureFlags,
    authz,
    searchValue,
    statusFilter,
    templateFilter,
    triggerFilter,
    executions,
    filteredExecutions,
    templateOptions,
    triggerOptions,
    setParam,
  } = data;

  return (
    <div className="card executions-toolbar">
      <div className="form-grid">
        <div>
          <label htmlFor="executions-search">Search</label>
          <input
            id="executions-search"
            type="text"
            placeholder="Search by id, goal, team, project, template, or trigger"
            value={searchValue}
            onChange={(event) => setParam('search', event.target.value)}
          />
        </div>
        <div>
          <label htmlFor="executions-status-filter">Status</label>
          <select id="executions-status-filter" value={statusFilter} onChange={(event) => setParam('status', event.target.value)}>
            <option value="all">All</option>
            <option value="active">Active</option>
            <option value="completed">Completed</option>
            <option value="failed">Failed</option>
            <option value="cancelled">Cancelled</option>
          </select>
        </div>
        <div>
          <label htmlFor="executions-template-filter">Template</label>
          <select id="executions-template-filter" value={templateFilter} onChange={(event) => setParam('template', event.target.value)}>
            <option value="all">All</option>
            {templateOptions.map((value) => (
              <option key={value} value={value}>{value}</option>
            ))}
          </select>
        </div>
        <div>
          <label htmlFor="executions-trigger-filter">Trigger</label>
          <select id="executions-trigger-filter" value={triggerFilter} onChange={(event) => setParam('trigger', event.target.value)}>
            <option value="all">All</option>
            {triggerOptions.map((value) => (
              <option key={value} value={value}>{value}</option>
            ))}
          </select>
        </div>
      </div>
      <p id="executions-summary" className="text-dim">
        {!featureFlags.remoteControlPlaneEnabled
          ? 'Remote control plane is disabled.'
          : !authz.permissions.viewExecutions
            ? 'Execution access is restricted for current role.'
            : filteredExecutions.length
              ? `Total: ${executions.length} · Visible: ${filteredExecutions.length}`
              : 'No executions match the current filters.'}
      </p>
    </div>
  );
}
