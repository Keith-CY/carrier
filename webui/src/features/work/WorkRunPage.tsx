import { Link } from 'react-router-dom';
import { labelizeWorkValue } from './model';
import { WorkCard, WorkMessage, WorkMetaList, WorkView } from './shared';
import { useWorkRunPageData } from './useWorkData';

export function WorkRunPage() {
  const data = useWorkRunPageData();
  const run = data.query.data?.run;

  return (
    <WorkView id="view-work-run" title={run?.id || 'Work Run'} backTo="/work" onRefresh={data.refresh}>
      {data.query.isLoading ? <WorkCard title="Loading"><WorkMessage text="Loading work run…" /></WorkCard> : null}
      {data.query.isError ? <WorkCard title="Unavailable"><WorkMessage tone="error" text={`Failed to load work run: ${data.query.error instanceof Error ? data.query.error.message : 'unknown error'}`} /></WorkCard> : null}
      {run ? (
        <>
          <WorkCard title="Run Overview">
            <WorkMetaList
              rows={[
                {
                  label: 'Project',
                  value: data.query.data?.project
                    ? <Link to={`/work/projects/${encodeURIComponent(data.query.data.project.id)}`}>{data.query.data.project.name || data.query.data.project.id}</Link>
                    : run.projectId,
                },
                {
                  label: 'Item',
                  value: data.query.data?.item
                    ? <Link to={`/work/items/${encodeURIComponent(data.query.data.item.id)}`}>{data.query.data.item.title || data.query.data.item.id}</Link>
                    : run.workItemId,
                },
                {
                  label: 'Execution',
                  value: run.executionId ? <Link to={`/executions/${encodeURIComponent(run.executionId)}`}>{run.executionId}</Link> : 'Not attached',
                },
                { label: 'Phase', value: labelizeWorkValue(run.phase) },
                { label: 'Backend', value: labelizeWorkValue(run.backend) },
                { label: 'Verification', value: labelizeWorkValue(run.verificationStatus) },
                { label: 'Publish', value: labelizeWorkValue(run.publishStatus) },
                { label: 'Workspace ID', value: run.workspaceId },
                { label: 'Workspace Path', value: run.workspacePath },
                { label: 'Workflow Digest', value: run.workflowDigest },
                { label: 'Lease Owner', value: run.leaseOwner },
                { label: 'Lease Expires', value: run.leaseExpiresAt },
                { label: 'Updated At', value: run.updatedAt },
              ]}
            />
          </WorkCard>

          <WorkCard title="Actions">
            <div className="btn-row">
              {run.executionId ? (
                <Link id="work-run-open-execution" to={`/executions/${encodeURIComponent(run.executionId)}`} className="btn btn-secondary btn-sm">
                  Open Execution
                </Link>
              ) : null}
              {run.executionId ? (
                <a
                  id="work-run-open-evidence"
                  href={`/api/v1/orchestrator/executions/${encodeURIComponent(run.executionId)}/evidence?format=json`}
                  className="btn btn-secondary btn-sm"
                >
                  Open Evidence
                </a>
              ) : null}
              <button id="work-run-resume" type="button" className="btn-sm btn-secondary" onClick={() => data.resumeMutation.mutate()}>
                Resume
              </button>
              <button id="work-run-cancel" type="button" className="btn-sm btn-secondary" onClick={() => data.cancelMutation.mutate()}>
                Cancel
              </button>
              <button id="work-run-reclaim" type="button" className="btn-sm btn-secondary" onClick={() => data.reclaimMutation.mutate()}>
                Reclaim
              </button>
              <button id="work-run-cleanup" type="button" className="btn-sm btn-secondary" onClick={() => data.cleanupMutation.mutate()}>
                Cleanup Workspace
              </button>
            </div>
            {data.resumeMutation.isError ? <WorkMessage tone="error" text={`Resume failed: ${data.resumeMutation.error instanceof Error ? data.resumeMutation.error.message : 'unknown error'}`} /> : null}
            {data.cancelMutation.isError ? <WorkMessage tone="error" text={`Cancel failed: ${data.cancelMutation.error instanceof Error ? data.cancelMutation.error.message : 'unknown error'}`} /> : null}
            {data.reclaimMutation.isError ? <WorkMessage tone="error" text={`Reclaim failed: ${data.reclaimMutation.error instanceof Error ? data.reclaimMutation.error.message : 'unknown error'}`} /> : null}
            {data.cleanupMutation.isError ? <WorkMessage tone="error" text={`Cleanup failed: ${data.cleanupMutation.error instanceof Error ? data.cleanupMutation.error.message : 'unknown error'}`} /> : null}
          </WorkCard>
        </>
      ) : null}
    </WorkView>
  );
}
