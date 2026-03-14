import { Link, useNavigate } from 'react-router-dom';
import { labelizeWorkValue } from './model';
import { WorkCard, WorkList, WorkMessage, WorkMetaList, WorkView } from './shared';
import { useWorkItemPageData } from './useWorkData';

export function WorkItemPage() {
  const data = useWorkItemPageData();
  const item = data.query.data?.item;
  const navigate = useNavigate();

  return (
    <WorkView id="view-work-item" title={item?.title || 'Work Item'} backTo="/work" onRefresh={data.refresh}>
      {data.query.isLoading ? <WorkCard title="Loading"><WorkMessage text="Loading work item…" /></WorkCard> : null}
      {data.query.isError ? <WorkCard title="Unavailable"><WorkMessage tone="error" text={`Failed to load work item: ${data.query.error instanceof Error ? data.query.error.message : 'unknown error'}`} /></WorkCard> : null}
      {item ? (
        <>
          <WorkCard title="Item Overview">
            <WorkMetaList
              rows={[
                {
                  label: 'Project',
                  value: data.query.data?.project
                    ? <Link to={`/work/projects/${encodeURIComponent(data.query.data.project.id)}`}>{data.query.data.project.name || data.query.data.project.id}</Link>
                    : item.projectId,
                },
                { label: 'State', value: labelizeWorkValue(item.state) },
                { label: 'Priority', value: labelizeWorkValue(item.priority) },
                { label: 'Source', value: `${item.source || 'local'} · ${item.sourceRef || 'n/a'}` },
                {
                  label: 'Latest Run',
                  value: data.query.data?.latestRun
                    ? <Link to={`/work/runs/${encodeURIComponent(data.query.data.latestRun.id)}`}>{data.query.data.latestRun.id}</Link>
                    : item.latestRunId,
                },
                { label: 'Labels', value: item.labels.join(', ') },
                { label: 'Updated At', value: item.updatedAt },
              ]}
            />
          </WorkCard>

          <WorkCard title="Actions">
            <div className="btn-row">
              <button
                id="work-item-start-run"
                type="button"
                className="btn-sm btn-secondary"
                onClick={async () => {
                  const payload = await data.startRunMutation.mutateAsync('local_sandboxed');
                  const runId = String(payload?.run?.id || '').trim();
                  if (runId) navigate(`/work/runs/${encodeURIComponent(runId)}`);
                }}
              >
                Start Run
              </button>
              <button
                id="work-item-resume-run"
                type="button"
                className="btn-sm btn-secondary"
                disabled={!data.query.data?.latestRun}
                onClick={async () => {
                  const payload = await data.resumeLatestRunMutation.mutateAsync();
                  const runId = String(payload?.run?.id || data.query.data?.latestRun?.id || '').trim();
                  if (runId) navigate(`/work/runs/${encodeURIComponent(runId)}`);
                }}
              >
                Resume
              </button>
              <button id="work-item-cancel" type="button" className="btn-sm btn-secondary" onClick={() => data.cancelMutation.mutate()}>
                Cancel Item
              </button>
              <button id="work-item-complete" type="button" className="btn-sm btn-secondary" onClick={() => data.completeMutation.mutate()}>
                Mark Done
              </button>
            </div>
            {data.resumeLatestRunMutation.isError ? <WorkMessage tone="error" text={`Resume failed: ${data.resumeLatestRunMutation.error instanceof Error ? data.resumeLatestRunMutation.error.message : 'unknown error'}`} /> : null}
            {data.startRunMutation.isError ? <WorkMessage tone="error" text={`Start failed: ${data.startRunMutation.error instanceof Error ? data.startRunMutation.error.message : 'unknown error'}`} /> : null}
            {data.cancelMutation.isError ? <WorkMessage tone="error" text={`Cancel failed: ${data.cancelMutation.error instanceof Error ? data.cancelMutation.error.message : 'unknown error'}`} /> : null}
            {data.completeMutation.isError ? <WorkMessage tone="error" text={`Complete failed: ${data.completeMutation.error instanceof Error ? data.completeMutation.error.message : 'unknown error'}`} /> : null}
          </WorkCard>

          {item.description ? (
            <WorkCard title="Description">
              <p className="execution-detail-line" style={{ color: 'var(--text)' }}>{item.description}</p>
            </WorkCard>
          ) : null}

          <WorkCard title="Acceptance">
            {item.acceptance.length ? (
              <WorkList>
                {item.acceptance.map((criterion) => (
                  <li key={criterion} className="execution-detail-line" style={{ color: 'var(--text)' }}>{criterion}</li>
                ))}
              </WorkList>
            ) : <WorkMessage text="No acceptance criteria recorded for this item." />}
          </WorkCard>
        </>
      ) : null}
    </WorkView>
  );
}
