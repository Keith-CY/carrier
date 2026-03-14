export function ExecutionWorkContextBlock({ execution }: { execution: any }) {
  const mode = String(execution?.mode || '').trim().toLowerCase();
  const work = execution?.work && typeof execution.work === 'object' ? execution.work : {};
  const projectId = String(work?.projectId || '').trim();
  const workItemId = String(work?.workItemId || '').trim();
  const runId = String(work?.runId || '').trim();
  const workspaceId = String(work?.workspaceId || '').trim();
  const workspacePath = String(work?.workspacePath || '').trim();
  const backend = String(work?.backend || '').trim();
  const workflowDigest = String(work?.workflowDigest || '').trim();
  const phase = String(work?.phase || '').trim();
  const verificationStatus = String(work?.verificationStatus || '').trim();
  const publishStatus = String(work?.publishStatus || '').trim();

  if (mode !== 'work' && !projectId && !workItemId && !runId && !workspaceId && !workspacePath && !backend && !workflowDigest && !phase && !verificationStatus && !publishStatus) {
    return null;
  }

  return (
    <div className="execution-detail-block">
      <div className="execution-detail-title">Work Context</div>
      <div className="execution-detail-line">mode: {mode || 'work'}</div>
      {projectId ? <div className="execution-detail-line">project: {projectId}</div> : null}
      {workItemId ? <div className="execution-detail-line">work item: {workItemId}</div> : null}
      {runId ? <div className="execution-detail-line">run: {runId}</div> : null}
      {workspaceId ? <div className="execution-detail-line">workspace: {workspaceId}</div> : null}
      {workspacePath ? <div className="execution-detail-line">workspace path: {workspacePath}</div> : null}
      {backend ? <div className="execution-detail-line">backend: {backend}</div> : null}
      {phase ? <div className="execution-detail-line">phase: {phase}</div> : null}
      {verificationStatus ? <div className="execution-detail-line">verification: {verificationStatus}</div> : null}
      {publishStatus ? <div className="execution-detail-line">publish: {publishStatus}</div> : null}
      {workflowDigest ? <div className="execution-detail-line">workflow: {workflowDigest}</div> : null}
    </div>
  );
}
