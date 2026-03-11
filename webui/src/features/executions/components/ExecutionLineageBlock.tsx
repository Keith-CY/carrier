export function ExecutionLineageBlock({ execution }: { execution: any }) {
  const parentExecutionID = String(execution?.parentExecutionId || '').trim();
  const sourceExecutionID = String(execution?.sourceExecutionId || '').trim();
  const launchReason = String(execution?.launchReason || '').trim();

  if (!(parentExecutionID || sourceExecutionID || launchReason)) return null;

  return (
    <div className="execution-detail-block">
      <div className="execution-detail-title">Execution Lineage</div>
      {parentExecutionID ? <div className="execution-detail-line">parent: {parentExecutionID}</div> : null}
      {sourceExecutionID ? <div className="execution-detail-line">source: {sourceExecutionID}</div> : null}
      {launchReason ? <div className="execution-detail-line">launch reason: {launchReason}</div> : null}
    </div>
  );
}
