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
  const mediaArtifacts = artifacts.filter((item: any) => String(item?.mediaType || '').trim() || String(item?.outputRole || '').trim() === 'generated');

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
      {mediaArtifacts.length ? <div className="execution-detail-line">Media Outputs</div> : null}
      {mediaArtifacts.map((item: any) => {
        const name = String(item?.name || item?.id || 'media output').trim();
        const mediaParts = [];
        if (String(item?.mediaType || '').trim()) mediaParts.push(String(item.mediaType).trim());
        if (String(item?.outputRole || '').trim()) mediaParts.push(`role=${String(item.outputRole).trim()}`);
        if (String(item?.transport || '').trim()) mediaParts.push(`transport=${String(item.transport).trim()}`);
        if (String(item?.deliveryMethod || '').trim()) mediaParts.push(`method=${String(item.deliveryMethod).trim()}`);
        if (String(item?.previewText || '').trim()) mediaParts.push(`preview=${String(item.previewText).trim()}`);
        if (String(item?.downloadUrl || '').trim()) mediaParts.push(`delivery=${String(item.downloadUrl).trim()}`);
        else if (String(item?.externalId || '').trim()) mediaParts.push(`delivery=${String(item.externalId).trim()}`);
        else if (String(item?.path || '').trim()) mediaParts.push(`delivery=${String(item.path).trim()}`);
        if (String(item?.attachmentId || '').trim()) mediaParts.push(`attachment=${String(item.attachmentId).trim()}`);
        return (
          <div key={`media-${String(item?.id || name)}`}>
            <div className="execution-detail-line">{name}{mediaParts.length ? ` · ${mediaParts.join(' · ')}` : ''}</div>
          </div>
        );
      })}
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
        if (String(item?.transport || '').trim()) metaParts.push(`transport=${String(item.transport).trim()}`);
        if (String(item?.deliveryMethod || '').trim()) metaParts.push(`method=${String(item.deliveryMethod).trim()}`);
        if (String(item?.previewText || '').trim()) metaParts.push(`preview=${String(item.previewText).trim()}`);
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
