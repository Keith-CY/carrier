import { formatDateTime, toFiniteNumber } from '../../../lib/format';
import { artifactDownloadPath } from './detailShared';

export function ExecutionOutcomeBlock({
  execution,
  onDownloadArtifact,
}: {
  execution: any;
  onDownloadArtifact: (artifactId: string, filename: string) => void | Promise<void>;
}) {
  const outcome = execution?.outcome && typeof execution.outcome === 'object' ? execution.outcome : {};
  const artifacts = Array.isArray(outcome?.artifacts) ? outcome.artifacts : [];

  if (!(String(outcome?.summary || '').trim() || String(outcome?.failureReason || '').trim() || String(outcome?.failureCategory || '').trim() || String(outcome?.renderMode || '').trim() || artifacts.length)) {
    return null;
  }

  return (
    <div className="execution-detail-block">
      <div className="execution-detail-title">Outcome</div>
      {String(outcome?.summary || '').trim() ? <div className="execution-detail-line">Summary: {String(outcome.summary).trim()}</div> : null}
      {String(outcome?.failureReason || '').trim() ? <div className="execution-detail-line">Failure reason: {String(outcome.failureReason).trim()}</div> : null}
      {String(outcome?.failureCategory || '').trim() ? <div className="execution-detail-line">Failure category: {String(outcome.failureCategory).trim()}</div> : null}
      {String(outcome?.renderMode || '').trim() ? <div className="execution-detail-line">Render mode: {String(outcome.renderMode).trim()}</div> : null}
      {artifacts.length ? <div className="execution-detail-line">Artifacts</div> : null}
      {artifacts.map((item: any) => {
        const artifactID = String(item?.id || '').trim();
        const name = String(item?.name || artifactID).trim();
        const metaParts = [];
        if (String(item?.kind || '').trim()) metaParts.push(String(item.kind).trim());
        if (String(item?.outputRole || '').trim()) metaParts.push(`role=${String(item.outputRole).trim()}`);
        if (String(item?.contentType || '').trim()) metaParts.push(String(item.contentType).trim());
        if (String(item?.mediaType || '').trim()) metaParts.push(`media=${String(item.mediaType).trim()}`);
        if (String(item?.source || '').trim()) metaParts.push(`source=${String(item.source).trim()}`);
        if (String(item?.externalId || '').trim()) metaParts.push(`external=${String(item.externalId).trim()}`);
        if (String(item?.attachmentId || '').trim()) metaParts.push(`attachment=${String(item.attachmentId).trim()}`);
        if (String(item?.downloadUrl || '').trim()) metaParts.push(String(item.downloadUrl).trim());
        if (toFiniteNumber(item?.sizeBytes, 0) > 0) metaParts.push(`${toFiniteNumber(item.sizeBytes, 0)} bytes`);
        if (String(item?.createdAt || '').trim()) metaParts.push(formatDateTime(item.createdAt));
        return (
          <div key={artifactID || name}>
            <div className="execution-detail-line">{name}{metaParts.length ? ` · ${metaParts.join(' · ')}` : ''}</div>
            {artifactID ? (
              <a
                className="btn-sm btn-secondary"
                href={artifactDownloadPath(String(execution?.id || '').trim(), artifactID)}
                onClick={(event) => {
                  event.preventDefault();
                  void onDownloadArtifact(artifactID, name);
                }}
              >
                Download {name}
              </a>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}
