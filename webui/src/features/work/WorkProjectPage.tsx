import { useState } from 'react';
import { Link } from 'react-router-dom';
import { labelizeWorkValue } from './model';
import { WorkCard, WorkList, WorkMessage, WorkMetaList, WorkView } from './shared';
import { useWorkProjectPageData } from './useWorkData';

export function WorkProjectPage() {
  const data = useWorkProjectPageData();
  const project = data.query.data?.project;
  const [newTitle, setNewTitle] = useState('');
  const [newDescription, setNewDescription] = useState('');
  const [newPriority, setNewPriority] = useState('normal');
  const [repository, setRepository] = useState('');
  const [issueNumber, setIssueNumber] = useState('');
  const [pullRequestNumber, setPullRequestNumber] = useState('');

  return (
    <WorkView id="view-work-project" title={project?.name || 'Work Project'} backTo="/work" onRefresh={data.refresh}>
      {data.query.isLoading ? <WorkCard title="Loading"><WorkMessage text="Loading work project…" /></WorkCard> : null}
      {data.query.isError ? <WorkCard title="Unavailable"><WorkMessage tone="error" text={`Failed to load work project: ${data.query.error instanceof Error ? data.query.error.message : 'unknown error'}`} /></WorkCard> : null}
      {project ? (
        <>
          <WorkCard title="Project Overview">
            <WorkMetaList
              rows={[
                { label: 'State', value: labelizeWorkValue(project.state) },
                { label: 'Source', value: `${project.sourceType || 'source'} · ${project.sourceRef || 'n/a'}` },
                { label: 'Default Branch', value: project.defaultBranch },
                { label: 'Workflow Path', value: project.workflowPath },
                { label: 'Workflow Digest', value: project.workflowDigest },
                { label: 'Last Sync', value: project.lastSyncAt },
                { label: 'Sync Error', value: project.lastSyncError },
              ]}
            />
          </WorkCard>

          <WorkCard title="Project Actions">
            <div className="btn-row" style={{ marginBottom: 12 }}>
              <button
                id="work-project-sync"
                type="button"
                className="btn-sm btn-secondary"
                onClick={() => data.syncMutation.mutate()}
              >
                Sync Project
              </button>
            </div>
            {data.syncMutation.isError ? (
              <WorkMessage tone="error" text={`Sync failed: ${data.syncMutation.error instanceof Error ? data.syncMutation.error.message : 'unknown error'}`} />
            ) : null}

            <div className="card" style={{ marginBottom: 12 }}>
              <div className="section-head"><h4>Create Local Work Item</h4></div>
              <div className="form-grid">
                <div>
                  <label htmlFor="work-project-new-title">Title</label>
                  <input id="work-project-new-title" type="text" value={newTitle} onChange={(event) => setNewTitle(event.target.value)} placeholder="Investigate worker drift" />
                </div>
                <div>
                  <label htmlFor="work-project-new-priority">Priority</label>
                  <select id="work-project-new-priority" value={newPriority} onChange={(event) => setNewPriority(event.target.value)}>
                    <option value="low">low</option>
                    <option value="normal">normal</option>
                    <option value="high">high</option>
                    <option value="urgent">urgent</option>
                  </select>
                </div>
                <div style={{ gridColumn: '1 / -1' }}>
                  <label htmlFor="work-project-new-description">Description</label>
                  <input id="work-project-new-description" type="text" value={newDescription} onChange={(event) => setNewDescription(event.target.value)} placeholder="Document the failure mode and mitigation." />
                </div>
              </div>
              <div className="btn-row" style={{ marginTop: 12 }}>
                <button
                  id="work-project-create-item"
                  type="button"
                  className="btn-sm btn-secondary"
                  onClick={() => data.createItemMutation.mutate({ title: newTitle, description: newDescription, priority: newPriority })}
                >
                  Create Work Item
                </button>
              </div>
              {data.createItemMutation.isError ? (
                <WorkMessage tone="error" text={`Create failed: ${data.createItemMutation.error instanceof Error ? data.createItemMutation.error.message : 'unknown error'}`} />
              ) : null}
            </div>

            <div className="card">
              <div className="section-head"><h4>Import GitHub Work Item</h4></div>
              <div className="form-grid">
                <div>
                  <label htmlFor="work-project-import-repository">Repository</label>
                  <input id="work-project-import-repository" type="text" value={repository} onChange={(event) => setRepository(event.target.value)} placeholder="owner/repo" />
                </div>
                <div>
                  <label htmlFor="work-project-import-issue">Issue Number</label>
                  <input id="work-project-import-issue" type="number" value={issueNumber} onChange={(event) => setIssueNumber(event.target.value)} placeholder="42" />
                </div>
                <div>
                  <label htmlFor="work-project-import-pr">Pull Request Number</label>
                  <input id="work-project-import-pr" type="number" value={pullRequestNumber} onChange={(event) => setPullRequestNumber(event.target.value)} placeholder="17" />
                </div>
              </div>
              <div className="btn-row" style={{ marginTop: 12 }}>
                <button
                  id="work-project-import-item"
                  type="button"
                  className="btn-sm btn-secondary"
                  onClick={() => data.importMutation.mutate({
                    repository,
                    issueNumber: issueNumber ? Number(issueNumber) : undefined,
                    pullRequestNumber: pullRequestNumber ? Number(pullRequestNumber) : undefined,
                  })}
                >
                  Import GitHub Work Item
                </button>
              </div>
              {data.importMutation.isError ? (
                <WorkMessage tone="error" text={`Import failed: ${data.importMutation.error instanceof Error ? data.importMutation.error.message : 'unknown error'}`} />
              ) : null}
            </div>
          </WorkCard>

          <WorkCard title="Items">
            {data.query.data?.items.length ? (
              <WorkList>
                {data.query.data.items.map((item) => (
                  <li key={item.id} className="execution-card">
                    <div className="section-head">
                      <Link to={`/work/items/${encodeURIComponent(item.id)}`}>{item.title || item.id}</Link>
                      <span className="execution-detail-line">{labelizeWorkValue(item.state)}</span>
                    </div>
                    <div className="execution-detail-line">{labelizeWorkValue(item.priority)} · {item.source || 'local'} · {item.sourceRef || 'n/a'}</div>
                  </li>
                ))}
              </WorkList>
            ) : <WorkMessage text="No work items are linked to this project." />}
          </WorkCard>

          <WorkCard title="Runs">
            {data.query.data?.runs.length ? (
              <WorkList>
                {data.query.data.runs.map((run) => (
                  <li key={run.id} className="execution-card">
                    <div className="section-head">
                      <Link to={`/work/runs/${encodeURIComponent(run.id)}`}>{run.id}</Link>
                      <span className="execution-detail-line">{labelizeWorkValue(run.phase)}</span>
                    </div>
                    <div className="execution-detail-line">{labelizeWorkValue(run.backend)} · {labelizeWorkValue(run.publishStatus)}</div>
                  </li>
                ))}
              </WorkList>
            ) : <WorkMessage text="No runs have been created for this project." />}
          </WorkCard>
        </>
      ) : null}
    </WorkView>
  );
}
