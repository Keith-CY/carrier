import type { useProvidersData } from '../useProvidersData';

type ProvidersData = ReturnType<typeof useProvidersData>;

export function ProviderBindingsList({ data }: { data: ProvidersData }) {
  const { bindings, canManageProviders, deleteBindingMutation } = data;

  return (
    <div id="bindings-list" className="agent-grid" style={{ marginTop: '12px' }}>
      {bindings.length ? bindings.map((binding: any) => {
        const bindingId = String(binding?.id || '').trim();
        return (
          <div key={bindingId} className="agent-card">
            <h4>{String(binding?.targetType || 'target')}: {String(binding?.targetId || '')}</h4>
            <div className="instance-meta">profile: {String(binding?.profileId || '')}</div>
            {canManageProviders ? <div className="btn-row"><button type="button" className="btn-sm btn-danger" onClick={() => { if (window.confirm('Delete provider binding?')) deleteBindingMutation.mutate(bindingId); }}>Delete</button></div> : null}
          </div>
        );
      }) : <div className="card">No provider bindings configured.</div>}
    </div>
  );
}
