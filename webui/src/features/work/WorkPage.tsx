import { Link } from 'react-router-dom';
import { labelizeWorkValue } from './model';
import { WorkCard, WorkList, WorkMessage, WorkMetaList, WorkView } from './shared';
import { useWorkPageData } from './useWorkData';

export function WorkPage() {
  const data = useWorkPageData();

  return (
    <WorkView id="view-work" title="Work" onRefresh={data.refresh}>
      {data.query.isLoading ? <WorkCard title="Loading"><WorkMessage text="Loading work projects, items, and runs…" /></WorkCard> : null}
      {data.query.isError ? <WorkCard title="Unavailable"><WorkMessage tone="error" text={`Failed to load work: ${data.query.error instanceof Error ? data.query.error.message : 'unknown error'}`} /></WorkCard> : null}
      {data.query.data ? (
        <>
          <WorkCard title="Overview">
            <WorkMetaList
              rows={[
                { label: 'Projects', value: String(data.query.data.projects.length) },
                { label: 'Items', value: String(data.query.data.items.length) },
                { label: 'Runs', value: String(data.query.data.runs.length) },
              ]}
            />
          </WorkCard>

          <WorkCard title="Projects">
            {data.query.data.projects.length ? (
              <WorkList>
                {data.query.data.projects.map((project) => (
                  <li key={project.id} className="execution-card">
                    <div className="section-head">
                      <Link to={`/work/projects/${encodeURIComponent(project.id)}`}>{project.name || project.id}</Link>
                      <span className="execution-detail-line">{labelizeWorkValue(project.state)}</span>
                    </div>
                    <div className="execution-detail-line">{project.sourceType || 'source'} · {project.sourceRef || 'n/a'}</div>
                    {project.workflowDigest ? <div className="execution-detail-line">{project.workflowDigest}</div> : null}
                  </li>
                ))}
              </WorkList>
            ) : <WorkMessage text="No work projects registered yet." />}
          </WorkCard>

          <WorkCard title="Queue">
            {data.query.data.items.length ? (
              <WorkList>
                {data.query.data.items.map((item) => (
                  <li key={item.id} className="execution-card">
                    <div className="section-head">
                      <Link to={`/work/items/${encodeURIComponent(item.id)}`}>{item.title || item.id}</Link>
                      <span className="execution-detail-line">{labelizeWorkValue(item.priority)}</span>
                    </div>
                    <div className="execution-detail-line">{labelizeWorkValue(item.state)} · project {item.projectId}</div>
                    {item.latestRunId ? <div className="execution-detail-line">latest run {item.latestRunId}</div> : null}
                  </li>
                ))}
              </WorkList>
            ) : <WorkMessage text="No work items available." />}
          </WorkCard>

          <WorkCard title="Active Runs">
            {data.query.data.runs.length ? (
              <WorkList>
                {data.query.data.runs.map((run) => (
                  <li key={run.id} className="execution-card">
                    <div className="section-head">
                      <Link to={`/work/runs/${encodeURIComponent(run.id)}`}>{run.id}</Link>
                      <span className="execution-detail-line">{labelizeWorkValue(run.phase)}</span>
                    </div>
                    <div className="execution-detail-line">{labelizeWorkValue(run.backend)} · {labelizeWorkValue(run.verificationStatus)}</div>
                    <div className="execution-detail-line">item {run.workItemId || 'n/a'} · project {run.projectId || 'n/a'}</div>
                  </li>
                ))}
              </WorkList>
            ) : <WorkMessage text="No work runs started yet." />}
          </WorkCard>
        </>
      ) : null}
    </WorkView>
  );
}
