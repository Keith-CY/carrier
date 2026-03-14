import { Link } from 'react-router-dom';
import { labelizeWorkValue } from './model';
import { WorkCard, WorkList, WorkMessage, WorkView } from './shared';
import { useWorkPageData } from './useWorkData';

function WorkProjectsSection() {
  const data = useWorkPageData();
  return (
    <WorkView id="view-work-projects" title="Work Projects" backTo="/work" onRefresh={data.refresh}>
      {data.query.isLoading ? <WorkCard title="Loading"><WorkMessage text="Loading work projects…" /></WorkCard> : null}
      {data.query.isError ? <WorkCard title="Unavailable"><WorkMessage tone="error" text={`Failed to load work projects: ${data.query.error instanceof Error ? data.query.error.message : 'unknown error'}`} /></WorkCard> : null}
      {data.query.data ? (
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
                </li>
              ))}
            </WorkList>
          ) : <WorkMessage text="No work projects registered yet." />}
        </WorkCard>
      ) : null}
    </WorkView>
  );
}

function WorkItemsSection() {
  const data = useWorkPageData();
  return (
    <WorkView id="view-work-items" title="Work Items" backTo="/work" onRefresh={data.refresh}>
      {data.query.isLoading ? <WorkCard title="Loading"><WorkMessage text="Loading work items…" /></WorkCard> : null}
      {data.query.isError ? <WorkCard title="Unavailable"><WorkMessage tone="error" text={`Failed to load work items: ${data.query.error instanceof Error ? data.query.error.message : 'unknown error'}`} /></WorkCard> : null}
      {data.query.data ? (
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
      ) : null}
    </WorkView>
  );
}

function WorkRunsSection() {
  const data = useWorkPageData();
  return (
    <WorkView id="view-work-runs" title="Work Runs" backTo="/work" onRefresh={data.refresh}>
      {data.query.isLoading ? <WorkCard title="Loading"><WorkMessage text="Loading work runs…" /></WorkCard> : null}
      {data.query.isError ? <WorkCard title="Unavailable"><WorkMessage tone="error" text={`Failed to load work runs: ${data.query.error instanceof Error ? data.query.error.message : 'unknown error'}`} /></WorkCard> : null}
      {data.query.data ? (
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
      ) : null}
    </WorkView>
  );
}

export { WorkProjectsSection as WorkProjectsPage, WorkItemsSection as WorkItemsPage, WorkRunsSection as WorkRunsPage };
