import { downloadFromAPI } from '../../lib/api';
import {
  ExecutionDetailContent,
  executionAttributionParts,
} from './ExecutionDetailContent';
import { useExecutionsData } from './useExecutionsData';

function evidenceDownloadPath(executionID: string): string {
  return `/api/v1/orchestrator/executions/${encodeURIComponent(String(executionID || '').trim())}/evidence?format=zip`;
}

function auditExportDownloadPath(executionID: string): string {
  return `/api/v1/audit/export?executionId=${encodeURIComponent(String(executionID || '').trim())}`;
}

export function ExecutionsPage() {
  const {
    featureFlags,
    authz,
    searchValue,
    statusFilter,
    templateFilter,
    triggerFilter,
    executions,
    detailQuery,
    filteredExecutions,
    templateOptions,
    triggerOptions,
    selectedExecutionId,
    setSelectedExecutionId,
    selectedExecution,
    selectedWorkers,
    selectedTerminal,
    selectedHasFailedTasks,
    selectedPolicyAskPending,
    cancelMutation,
    approveMutation,
    derivedMutation,
    refreshExecutions,
    setParam,
    navigate,
    executionCounts,
  } = useExecutionsData();

  return (
    <section id="view-executions" className="view">
      <div className="section-head">
        <h2>Executions</h2>
        <div className="section-actions">
          <button id="executions-refresh" className="btn-sm btn-secondary" onClick={() => void refreshExecutions()}>
            Refresh
          </button>
        </div>
      </div>

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

      <div className="executions-layout">
        <div className="card">
          <div id="executions-list" className="executions-list">
            {filteredExecutions.map((execution) => {
              const executionIdValue = String(execution?.id || '').trim();
              const counts = executionCounts(execution);
              return (
                <button
                  key={executionIdValue}
                  type="button"
                  className={`agent-card execution-card execution-list-card${selectedExecutionId === executionIdValue ? ' active' : ''}`}
                  onClick={() => {
                    setSelectedExecutionId(executionIdValue);
                    navigate(`/executions/${encodeURIComponent(executionIdValue)}`);
                  }}
                >
                  <div className="section-head">
                    <h4>{executionIdValue || 'execution'}</h4>
                    <span className={String(execution?.status || '').trim().toLowerCase() === 'completed' ? 'badge badge-ok' : 'badge badge-unknown'}>
                      {String(execution?.status || 'unknown').trim()}
                    </span>
                  </div>
                  <div className="execution-goal">{String(execution?.goal || '').trim() || '(no goal)'}</div>
                  <div className="instance-meta">
                    Tasks: {counts.taskUnits.length} · Completed: {counts.completed} · Failed: {counts.failed} · Updated: {String(execution?.updatedAt || '').trim() || 'n/a'}
                  </div>
                  {executionAttributionParts(execution).length ? <div className="instance-meta">{executionAttributionParts(execution).join(' · ')}</div> : null}
                </button>
              );
            })}
          </div>
        </div>

        <div className="card">
          <div className="section-head">
            <h3>Execution Detail</h3>
            <div className="section-actions">
              <button
                id="executions-export-evidence"
                className={`btn-sm btn-secondary${selectedExecution ? '' : ' hidden'}`}
                type="button"
                onClick={() => selectedExecutionId && void downloadFromAPI(evidenceDownloadPath(selectedExecutionId), `${selectedExecutionId}-evidence.zip`)}
              >
                Export Evidence
              </button>
              <button
                id="executions-export-audit"
                className={`btn-sm btn-secondary${selectedExecution ? '' : ' hidden'}`}
                type="button"
                onClick={() => selectedExecutionId && void downloadFromAPI(auditExportDownloadPath(selectedExecutionId), `${selectedExecutionId}-audit.json`)}
              >
                Export Audit
              </button>
              <button
                id="executions-retry"
                className={`btn-sm${authz.permissions.launchExecutions && selectedTerminal && selectedHasFailedTasks ? '' : ' hidden'}`}
                type="button"
                onClick={() => derivedMutation.mutate('retry')}
              >
                Retry Failed Tasks
              </button>
              <button
                id="executions-rerun"
                className={`btn-sm btn-secondary${authz.permissions.launchExecutions && selectedTerminal ? '' : ' hidden'}`}
                type="button"
                onClick={() => derivedMutation.mutate('rerun')}
              >
                Rerun
              </button>
              <button
                id="executions-clone"
                className={`btn-sm btn-secondary${authz.permissions.launchExecutions && selectedTerminal ? '' : ' hidden'}`}
                type="button"
                onClick={() => derivedMutation.mutate('clone')}
              >
                Clone
              </button>
              <button
                id="executions-policy-approve"
                className={`btn-sm${authz.permissions.approveExecutions && selectedPolicyAskPending ? '' : ' hidden'}`}
                type="button"
                onClick={() => approveMutation.mutate()}
              >
                Approve Policy &amp; Run
              </button>
              <button
                id="executions-cancel"
                className={`btn-sm btn-danger${authz.permissions.launchExecutions && !selectedTerminal && selectedExecution ? '' : ' hidden'}`}
                type="button"
                onClick={() => {
                  if (!selectedExecutionId || !window.confirm(`Cancel execution ${selectedExecutionId}?`)) return;
                  cancelMutation.mutate();
                }}
              >
                Cancel
              </button>
            </div>
          </div>
          <div id="executions-detail" className="execution-details-panel">
            {!selectedExecutionId
              ? 'Select an execution to inspect workers and task results.'
              : detailQuery.isLoading
                ? 'Loading details…'
                : detailQuery.isError
                  ? `Load failed: ${(detailQuery.error as Error).message}`
                  : selectedExecution
                    ? (
                      <ExecutionDetailContent
                        execution={selectedExecution}
                        workers={selectedWorkers}
                        onDownloadArtifact={(artifactId, filename) => downloadFromAPI(`/api/v1/orchestrator/executions/${encodeURIComponent(selectedExecutionId)}/artifacts/${encodeURIComponent(artifactId)}`, filename)}
                      />
                    )
                    : 'Execution details unavailable.'}
          </div>
        </div>
      </div>
    </section>
  );
}
