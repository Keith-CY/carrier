import { downloadFromAPI } from '../../../lib/api';
import { ExecutionDetailContent } from '../ExecutionDetailContent';
import type { ExecutionsData } from '../useExecutionsData';

function evidenceDownloadPath(executionID: string): string {
  return `/api/v1/orchestrator/executions/${encodeURIComponent(String(executionID || '').trim())}/evidence?format=zip`;
}

function auditExportDownloadPath(executionID: string): string {
  return `/api/v1/audit/export?executionId=${encodeURIComponent(String(executionID || '').trim())}`;
}

export function ExecutionDetailCard({ data }: { data: ExecutionsData }) {
  const {
    authz,
    detailQuery,
    metadataQuery,
    selectedExecutionId,
    selectedExecution,
    selectedExecutionMetadata,
    selectedWorkers,
    selectedTerminal,
    selectedHasFailedTasks,
    selectedPolicyAskPending,
    approveMutation,
    cancelMutation,
    derivedMutation,
  } = data;

  return (
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
              : metadataQuery.isError
                ? `Metadata load failed: ${(metadataQuery.error as Error).message}`
              : selectedExecution
                ? (
                  <ExecutionDetailContent
                    execution={selectedExecution}
                    metadata={selectedExecutionMetadata}
                    workers={selectedWorkers}
                    onDownloadArtifact={(artifactId, filename) => downloadFromAPI(`/api/v1/orchestrator/executions/${encodeURIComponent(selectedExecutionId)}/artifacts/${encodeURIComponent(artifactId)}`, filename)}
                  />
                )
                : 'Execution details unavailable.'}
      </div>
    </div>
  );
}
