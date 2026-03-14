import type { useProvidersData } from '../useProvidersData';

type ProvidersData = ReturnType<typeof useProvidersData>;

export function ResolutionPreviewCard({ data }: { data: ProvidersData }) {
  const { previewHostId, setPreviewHostId, previewAgentId, setPreviewAgentId, previewMutation, previewTextValue, hosts } = data;

  return (
    <div className="card" style={{ marginTop: '12px' }}>
      <h3>Resolution Preview</h3>
      <div className="form-grid">
        <div><label htmlFor="governance-preview-host">Host</label><select id="governance-preview-host" value={previewHostId} onChange={(event) => setPreviewHostId(event.target.value)}>{hosts.map((host: any) => <option key={String(host?.id || '')} value={String(host?.id || '')}>{String(host?.name || host?.id || '')}</option>)}</select></div>
        <div><label htmlFor="governance-preview-agent">Agent</label><input id="governance-preview-agent" type="text" value={previewAgentId} onChange={(event) => setPreviewAgentId(event.target.value)} placeholder="zeroclaw" /></div>
      </div>
      <div className="btn-row"><button id="governance-preview-resolve" className="btn-sm btn-secondary" onClick={() => previewMutation.mutate()}>Resolve</button></div>
      <div id="governance-preview-out" className="instance-meta" style={{ whiteSpace: 'pre-line', marginTop: '12px' }}>{previewTextValue}</div>
    </div>
  );
}
