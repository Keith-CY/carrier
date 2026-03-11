import type { useProvidersData } from '../useProvidersData';

type ProvidersData = ReturnType<typeof useProvidersData>;

export function ProviderBindingsCard({ data }: { data: ProvidersData }) {
  const {
    featureFlags,
    canManageProviders,
    bindingProfileId,
    setBindingProfileId,
    bindingTargetType,
    setBindingTargetType,
    bindingTargetId,
    setBindingTargetId,
    profileTestHostId,
    setProfileTestHostId,
    profiles,
    hosts,
    saveBindingMutation,
    message,
  } = data;

  return (
    <div className="card" style={{ marginTop: '12px' }}>
      <h3>Bind Profile</h3>
      <div className="form-grid">
        <div><label htmlFor="binding-profile-id">Profile</label><select id="binding-profile-id" value={bindingProfileId} disabled={!canManageProviders || !featureFlags.providerBindingEnabled} onChange={(event) => setBindingProfileId(event.target.value)}>{profiles.map((profile: any) => <option key={String(profile?.id || '')} value={String(profile?.id || '')}>{String(profile?.name || profile?.id || '')}</option>)}</select></div>
        <div><label htmlFor="binding-target-type">Target Type</label><select id="binding-target-type" value={bindingTargetType} disabled={!canManageProviders || !featureFlags.providerBindingEnabled} onChange={(event) => setBindingTargetType(event.target.value)}><option value="host">host</option><option value="instance">instance</option></select></div>
        <div><label htmlFor="binding-target-id">Target ID</label><input id="binding-target-id" type="text" value={bindingTargetId} disabled={!canManageProviders || !featureFlags.providerBindingEnabled} onChange={(event) => setBindingTargetId(event.target.value)} placeholder="hostId or hostId:agentId" /></div>
        <div><label htmlFor="profile-test-host">Profile Test Host</label><select id="profile-test-host" value={profileTestHostId} onChange={(event) => setProfileTestHostId(event.target.value)}><option value="">auto (first host)</option>{hosts.map((host: any) => <option key={String(host?.id || '')} value={String(host?.id || '')}>{String(host?.name || host?.id || '')}</option>)}</select></div>
      </div>
      <div className="btn-row">
        <button id="binding-save" disabled={!canManageProviders || !featureFlags.providerBindingEnabled} onClick={() => saveBindingMutation.mutate()}>Bind</button>
      </div>
      <div id="profiles-msg">
        {!featureFlags.providerBindingEnabled ? <p className="msg-info">Provider binding is disabled by feature flag.</p> : null}
        {message.text ? <p className={`msg-${message.type}`}>{message.text}</p> : null}
      </div>
    </div>
  );
}
