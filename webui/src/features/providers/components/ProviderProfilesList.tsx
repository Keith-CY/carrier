import type { useProvidersData } from '../useProvidersData';
import { normalizeBoolString } from '../shared';

type ProvidersData = ReturnType<typeof useProvidersData>;

export function ProviderProfilesList({ data }: { data: ProvidersData }) {
  const { profiles, canManageProviders, setEditingProfileId, setProfileForm, testProfileMutation, deleteProfileMutation } = data;

  return (
    <div id="profiles-list" className="agent-grid" style={{ marginTop: '12px' }}>
      {profiles.length ? profiles.map((profile: any) => {
        const profileId = String(profile?.id || '').trim();
        return (
          <div key={profileId} className="agent-card">
            <h4>{String(profile?.name || profileId)}</h4>
            <div className="instance-meta">{String(profile?.provider || '')}/{String(profile?.model || '')}</div>
            <div className="btn-row">
              <button type="button" className="btn-sm btn-secondary" onClick={() => {
                setEditingProfileId(profileId);
                setProfileForm({
                  name: String(profile?.name || ''),
                  provider: String(profile?.provider || ''),
                  model: String(profile?.model || ''),
                  baseUrl: String(profile?.baseUrl || ''),
                  authRef: String(profile?.authRef || ''),
                  enabled: normalizeBoolString(profile?.enabled),
                });
              }}>Edit</button>
              <button type="button" className="btn-sm btn-secondary" onClick={() => testProfileMutation.mutate(profileId)}>Test</button>
              {canManageProviders ? (
                <button type="button" className="btn-sm btn-danger" onClick={() => { if (window.confirm('Delete provider profile?')) deleteProfileMutation.mutate(profileId); }}>Delete</button>
              ) : null}
            </div>
          </div>
        );
      }) : <div className="card">No provider profiles configured.</div>}
    </div>
  );
}
