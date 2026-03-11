import type { useProvidersData } from '../useProvidersData';
import { EMPTY_PROFILE_FORM } from '../shared';

type ProvidersData = ReturnType<typeof useProvidersData>;

export function ProviderProfileEditor({ data }: { data: ProvidersData }) {
  const {
    profileForm,
    setProfileForm,
    editingProfileId,
    setEditingProfileId,
    canManageProviders,
    saveProfileMutation,
  } = data;

  return (
    <div className="card">
      <h3>Create Profile</h3>
      <div className="form-grid">
        <div><label htmlFor="profile-name">Name</label><input id="profile-name" type="text" value={profileForm.name} onChange={(event) => setProfileForm((current) => ({ ...current, name: event.target.value }))} placeholder="openai-gpt5" /></div>
        <div><label htmlFor="profile-provider">Provider</label><input id="profile-provider" type="text" value={profileForm.provider} onChange={(event) => setProfileForm((current) => ({ ...current, provider: event.target.value }))} placeholder="openai" /></div>
        <div><label htmlFor="profile-model">Model</label><input id="profile-model" type="text" value={profileForm.model} onChange={(event) => setProfileForm((current) => ({ ...current, model: event.target.value }))} placeholder="gpt-5" /></div>
        <div><label htmlFor="profile-base-url">Base URL</label><input id="profile-base-url" type="text" value={profileForm.baseUrl} onChange={(event) => setProfileForm((current) => ({ ...current, baseUrl: event.target.value }))} placeholder="https://api.openai.com/v1" /></div>
        <div><label htmlFor="profile-auth-ref">Auth Ref</label><input id="profile-auth-ref" type="text" value={profileForm.authRef} onChange={(event) => setProfileForm((current) => ({ ...current, authRef: event.target.value }))} placeholder="env:OPENAI_API_KEY" /></div>
        <div><label htmlFor="profile-enabled">Enabled</label><select id="profile-enabled" value={profileForm.enabled} onChange={(event) => setProfileForm((current) => ({ ...current, enabled: event.target.value }))}><option value="true">true</option><option value="false">false</option></select></div>
      </div>
      <div className="btn-row">
        <button id="profile-save" disabled={!canManageProviders} onClick={() => saveProfileMutation.mutate()}>{editingProfileId ? 'Update Profile' : 'Save Profile'}</button>
        <button id="profile-cancel-edit" className={`btn-sm btn-secondary${editingProfileId ? '' : ' hidden'}`} type="button" onClick={() => { setEditingProfileId(''); setProfileForm(EMPTY_PROFILE_FORM); }}>Cancel Edit</button>
      </div>
      <p id="profile-editor-state" className="text-dim">{editingProfileId ? `Editing profile: ${editingProfileId}` : ''}</p>
    </div>
  );
}
