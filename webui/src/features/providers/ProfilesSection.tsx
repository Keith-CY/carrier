import { useLocation } from 'react-router-dom';
import { parseCommaSeparatedValues } from '../../lib/format';
import {
  EMPTY_POLICY_FORM,
  EMPTY_PROFILE_FORM,
  EMPTY_TRIGGER_FORM,
  normalizeBoolString,
  renderInputsText,
  useProfilesData,
} from './useProfilesData';

export function ProfilesSection() {
  const location = useLocation();
  const mode = location.pathname.startsWith('/policies') ? 'policies' : 'providers';
  const {
    featureFlags,
    authz,
    message,
    setMessage,
    profileForm,
    setProfileForm,
    editingProfileId,
    setEditingProfileId,
    bindingTargetType,
    setBindingTargetType,
    bindingTargetId,
    setBindingTargetId,
    bindingProfileId,
    setBindingProfileId,
    profileTestHostId,
    setProfileTestHostId,
    previewHostId,
    setPreviewHostId,
    previewAgentId,
    setPreviewAgentId,
    previewTextValue,
    policyForm,
    setPolicyForm,
    triggerForm,
    setTriggerForm,
    editingTriggerId,
    setEditingTriggerId,
    canManageProviders,
    canManagePolicies,
    hosts,
    profiles,
    bindings,
    policies,
    templates,
    triggers,
    refreshAll,
    saveProfileMutation,
    deleteProfileMutation,
    testProfileMutation,
    saveBindingMutation,
    deleteBindingMutation,
    previewMutation,
    savePolicyMutation,
    deletePolicyMutation,
    saveTriggerMutation,
    toggleTriggerMutation,
    deleteTriggerMutation,
  } = useProfilesData(mode);

  const renderProviders = () => (
    <div id="providers-shell">
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

      <div className="card" style={{ marginTop: '12px' }}>
        <h3>Resolution Preview</h3>
        <div className="form-grid">
          <div><label htmlFor="governance-preview-host">Host</label><select id="governance-preview-host" value={previewHostId} onChange={(event) => setPreviewHostId(event.target.value)}>{hosts.map((host: any) => <option key={String(host?.id || '')} value={String(host?.id || '')}>{String(host?.name || host?.id || '')}</option>)}</select></div>
          <div><label htmlFor="governance-preview-agent">Agent</label><input id="governance-preview-agent" type="text" value={previewAgentId} onChange={(event) => setPreviewAgentId(event.target.value)} placeholder="zeroclaw" /></div>
        </div>
        <div className="btn-row"><button id="governance-preview-resolve" className="btn-sm btn-secondary" onClick={() => previewMutation.mutate()}>Resolve</button></div>
        <div id="governance-preview-out" className="instance-meta" style={{ whiteSpace: 'pre-line', marginTop: '12px' }}>{previewTextValue}</div>
      </div>

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
    </div>
  );

  const renderPolicies = () => (
    <div id="policies-shell">
      <div className="card" style={{ marginTop: '12px' }}>
        <h3>Execution Policies</h3>
        <div className="form-grid">
          <div><label htmlFor="execution-policy-name">Name</label><input id="execution-policy-name" type="text" value={policyForm.name} onChange={(event) => setPolicyForm((current) => ({ ...current, name: event.target.value }))} placeholder="review prod picoclaw runs" /></div>
          <div><label htmlFor="execution-policy-action">Action</label><select id="execution-policy-action" value={policyForm.action} onChange={(event) => setPolicyForm((current) => ({ ...current, action: event.target.value }))}><option value="ask">ask</option><option value="deny">deny</option><option value="allow">allow</option></select></div>
          <div><label htmlFor="execution-policy-priority">Priority</label><input id="execution-policy-priority" type="number" value={policyForm.priority} onChange={(event) => setPolicyForm((current) => ({ ...current, priority: event.target.value }))} min={0} max={1000} /></div>
          <div><label htmlFor="execution-policy-reason">Reason</label><input id="execution-policy-reason" type="text" value={policyForm.reason} onChange={(event) => setPolicyForm((current) => ({ ...current, reason: event.target.value }))} placeholder="operator review required" /></div>
          <div><label htmlFor="execution-policy-teams">Teams</label><input id="execution-policy-teams" type="text" value={policyForm.teams} onChange={(event) => setPolicyForm((current) => ({ ...current, teams: event.target.value }))} placeholder="platform, infra" /></div>
          <div><label htmlFor="execution-policy-projects">Projects</label><input id="execution-policy-projects" type="text" value={policyForm.projects} onChange={(event) => setPolicyForm((current) => ({ ...current, projects: event.target.value }))} placeholder="carrier, checkout" /></div>
          <div><label htmlFor="execution-policy-environments">Environments</label><input id="execution-policy-environments" type="text" value={policyForm.environments} onChange={(event) => setPolicyForm((current) => ({ ...current, environments: event.target.value }))} placeholder="prod, staging" /></div>
          <div><label htmlFor="execution-policy-template-ids">Template IDs</label><input id="execution-policy-template-ids" type="text" value={policyForm.templateIds} onChange={(event) => setPolicyForm((current) => ({ ...current, templateIds: event.target.value }))} placeholder="rollout-smoke, incident-triage" /></div>
          <div><label htmlFor="execution-policy-providers">Requested Providers</label><input id="execution-policy-providers" type="text" value={policyForm.providers} onChange={(event) => setPolicyForm((current) => ({ ...current, providers: event.target.value }))} placeholder="anthropic, openrouter" /></div>
          <div><label htmlFor="execution-policy-host-ids">Host IDs</label><input id="execution-policy-host-ids" type="text" value={policyForm.hostIds} onChange={(event) => setPolicyForm((current) => ({ ...current, hostIds: event.target.value }))} placeholder="host-1, host-2" /></div>
          <div><label htmlFor="execution-policy-host-labels">Host Labels</label><input id="execution-policy-host-labels" type="text" value={policyForm.hostLabels} onChange={(event) => setPolicyForm((current) => ({ ...current, hostLabels: event.target.value }))} placeholder="prod, gpu" /></div>
          <div><label htmlFor="execution-policy-agent-ids">Agent IDs</label><input id="execution-policy-agent-ids" type="text" value={policyForm.agentIds} onChange={(event) => setPolicyForm((current) => ({ ...current, agentIds: event.target.value }))} placeholder="picoclaw, zeroclaw" /></div>
          <div><label htmlFor="execution-policy-allowed-tools">Allowed Tools</label><input id="execution-policy-allowed-tools" type="text" value={policyForm.allowedTools} onChange={(event) => setPolicyForm((current) => ({ ...current, allowedTools: event.target.value }))} placeholder="grep, shell" /></div>
          <div><label htmlFor="execution-policy-max-timeout-ms">Max Task Timeout (ms)</label><input id="execution-policy-max-timeout-ms" type="number" value={policyForm.maxTimeoutMs} onChange={(event) => setPolicyForm((current) => ({ ...current, maxTimeoutMs: event.target.value }))} placeholder="45000" /></div>
          <div><label htmlFor="execution-policy-max-retry-budget">Max Retry Budget</label><input id="execution-policy-max-retry-budget" type="number" value={policyForm.maxRetryBudget} onChange={(event) => setPolicyForm((current) => ({ ...current, maxRetryBudget: event.target.value }))} placeholder="1" /></div>
          <div><label htmlFor="execution-policy-enabled">Enabled</label><select id="execution-policy-enabled" value={policyForm.enabled} onChange={(event) => setPolicyForm((current) => ({ ...current, enabled: event.target.value }))}><option value="true">true</option><option value="false">false</option></select></div>
        </div>
        <div className="btn-row"><button id="execution-policy-save" disabled={!canManagePolicies} onClick={() => savePolicyMutation.mutate()}>Save Policy</button></div>
      </div>

      <div className="card" style={{ marginTop: '12px' }}>
        <h3>Execution Triggers</h3>
        <div id="trigger-editor-state" className="text-dim" style={{ marginBottom: '8px' }}>{editingTriggerId ? `Editing trigger: ${editingTriggerId}` : ''}</div>
        <div className="form-grid">
          <div><label htmlFor="trigger-name">Name</label><input id="trigger-name" type="text" value={triggerForm.name} onChange={(event) => setTriggerForm((current) => ({ ...current, name: event.target.value }))} placeholder="incident webhook" /></div>
          <div><label htmlFor="trigger-type">Type</label><select id="trigger-type" value={triggerForm.type} onChange={(event) => setTriggerForm((current) => ({ ...current, type: event.target.value }))}><option value="webhook">webhook</option><option value="github">github</option><option value="schedule">schedule</option></select></div>
          <div><label htmlFor="trigger-template-id">Template</label><select id="trigger-template-id" value={triggerForm.templateId} onChange={(event) => setTriggerForm((current) => ({ ...current, templateId: event.target.value }))}>{templates.map((template: any) => <option key={String(template?.id || '')} value={String(template?.id || '')}>{String(template?.name || template?.id || '')}</option>)}</select></div>
          <div><label htmlFor="trigger-provider">Provider</label><input id="trigger-provider" type="text" value={triggerForm.provider} onChange={(event) => setTriggerForm((current) => ({ ...current, provider: event.target.value }))} placeholder="openrouter" /></div>
          <div><label htmlFor="trigger-host-ids">Host IDs</label><input id="trigger-host-ids" type="text" value={triggerForm.hostIds} onChange={(event) => setTriggerForm((current) => ({ ...current, hostIds: event.target.value }))} placeholder="host-1, host-2" /></div>
          <div><label htmlFor="trigger-host-labels">Host Labels</label><input id="trigger-host-labels" type="text" value={triggerForm.hostLabels} onChange={(event) => setTriggerForm((current) => ({ ...current, hostLabels: event.target.value }))} placeholder="prod, gpu" /></div>
          <div><label htmlFor="trigger-max-concurrency">Max Concurrency</label><input id="trigger-max-concurrency" type="number" value={triggerForm.maxConcurrency} onChange={(event) => setTriggerForm((current) => ({ ...current, maxConcurrency: event.target.value }))} placeholder="2" /></div>
          <div><label htmlFor="trigger-timezone">Timezone</label><input id="trigger-timezone" type="text" value={triggerForm.timezone} onChange={(event) => setTriggerForm((current) => ({ ...current, timezone: event.target.value }))} placeholder="UTC" /></div>
          <div><label htmlFor="trigger-webhook-secret">Webhook Secret</label><input id="trigger-webhook-secret" type="text" value={triggerForm.webhookSecret} onChange={(event) => setTriggerForm((current) => ({ ...current, webhookSecret: event.target.value }))} placeholder="secret" /></div>
          <div><label htmlFor="trigger-github-command">GitHub Command</label><input id="trigger-github-command" type="text" value={triggerForm.githubCommand} onChange={(event) => setTriggerForm((current) => ({ ...current, githubCommand: event.target.value }))} placeholder="/carrier triage" /></div>
          <div><label htmlFor="trigger-github-label">GitHub Label</label><input id="trigger-github-label" type="text" value={triggerForm.githubLabel} onChange={(event) => setTriggerForm((current) => ({ ...current, githubLabel: event.target.value }))} placeholder="needs-triage" /></div>
          <div><label htmlFor="trigger-github-repository">GitHub Repository</label><input id="trigger-github-repository" type="text" value={triggerForm.githubRepository} onChange={(event) => setTriggerForm((current) => ({ ...current, githubRepository: event.target.value }))} placeholder="Keith-CY/carrier" /></div>
          <div><label htmlFor="trigger-cron">Cron</label><input id="trigger-cron" type="text" value={triggerForm.cron} onChange={(event) => setTriggerForm((current) => ({ ...current, cron: event.target.value }))} placeholder="0 * * * *" /></div>
          <div><label htmlFor="trigger-policy-approve">Policy Approve</label><input id="trigger-policy-approve" type="checkbox" checked={triggerForm.policyApprove} onChange={(event) => setTriggerForm((current) => ({ ...current, policyApprove: event.target.checked }))} /></div>
          <div style={{ gridColumn: '1 / -1' }}><label htmlFor="trigger-inputs">Inputs</label><textarea id="trigger-inputs" rows={4} value={triggerForm.inputs} onChange={(event) => setTriggerForm((current) => ({ ...current, inputs: event.target.value }))} placeholder={'service={{payload.service}}\nenvironment={{payload.environment}}'} /></div>
        </div>
        <div className="btn-row">
          <button id="trigger-save" disabled={!canManagePolicies} onClick={() => saveTriggerMutation.mutate()}>{editingTriggerId ? 'Update Trigger' : 'Save Trigger'}</button>
          <button id="trigger-cancel-edit" className={`btn-sm btn-secondary${editingTriggerId ? '' : ' hidden'}`} type="button" onClick={() => { setEditingTriggerId(''); setTriggerForm((current) => ({ ...EMPTY_TRIGGER_FORM, templateId: current.templateId || String(templates[0]?.id || '').trim() })); }}>Cancel Edit</button>
        </div>
      </div>

      <div id="profiles-msg">
        {message.text ? <p className={`msg-${message.type}`}>{message.text}</p> : null}
      </div>

      <div id="execution-policies-list" className="agent-grid" style={{ marginTop: '12px' }}>
        {policies.length ? policies.map((policy: any) => {
          const policyId = String(policy?.id || '').trim();
          const teams = parseCommaSeparatedValues(Array.isArray(policy?.teams) ? policy.teams.join(', ') : String(policy?.teams || '')).sort((left, right) => left.localeCompare(right));
          const projects = parseCommaSeparatedValues(Array.isArray(policy?.projects) ? policy.projects.join(', ') : String(policy?.projects || '')).sort((left, right) => left.localeCompare(right));
          const environments = parseCommaSeparatedValues(Array.isArray(policy?.environments) ? policy.environments.join(', ') : String(policy?.environments || '')).sort((left, right) => left.localeCompare(right));
          const templateIds = parseCommaSeparatedValues(Array.isArray(policy?.templateIds) ? policy.templateIds.join(', ') : String(policy?.templateIds || '')).sort((left, right) => left.localeCompare(right));
          const hostLabels = parseCommaSeparatedValues(Array.isArray(policy?.hostLabels) ? policy.hostLabels.join(', ') : String(policy?.hostLabels || '')).sort((left, right) => left.localeCompare(right));
          const allowedTools = parseCommaSeparatedValues(Array.isArray(policy?.allowedTools) ? policy.allowedTools.join(', ') : String(policy?.allowedTools || '')).sort((left, right) => left.localeCompare(right));
          return (
            <div key={policyId} className="agent-card">
              <h4>{String(policy?.name || policyId)}</h4>
              <div className="instance-meta">
                <div>{String(policy?.action || 'ask')}</div>
                {teams.length ? <div>teams: {teams.join(', ')}</div> : null}
                {projects.length ? <div>projects: {projects.join(', ')}</div> : null}
                {environments.length ? <div>environments: {environments.join(', ')}</div> : null}
                {templateIds.length ? <div>templates: {templateIds.join(', ')}</div> : null}
                {hostLabels.length ? <div>host labels: {hostLabels.join(', ')}</div> : null}
                {allowedTools.length ? <div>allowed tools: {allowedTools.join(', ')}</div> : null}
                {policy?.maxTaskTimeoutMs ? <div>max timeout: {String(policy.maxTaskTimeoutMs)}ms</div> : null}
                {policy?.maxRetryBudget != null ? <div>max retry: {String(policy.maxRetryBudget)}</div> : null}
              </div>
              {canManagePolicies ? <div className="btn-row"><button type="button" className="btn-sm btn-danger" onClick={() => { if (window.confirm('Delete execution policy?')) deletePolicyMutation.mutate(policyId); }}>Delete</button></div> : null}
            </div>
          );
        }) : <div className="card">No execution policies configured.</div>}
      </div>

      <div id="execution-triggers-list" className="agent-grid" style={{ marginTop: '12px' }}>
        {triggers.length ? triggers.map((trigger: any) => {
          const triggerId = String(trigger?.id || '').trim();
          const config = trigger?.config && typeof trigger.config === 'object' ? trigger.config : {};
          const enabled = trigger?.enabled !== false;
          return (
            <div key={triggerId} className="agent-card">
              <h4>{String(trigger?.name || triggerId)}</h4>
              <div className="instance-meta">
                <div>type: {String(trigger?.type || 'unknown')}</div>
                <div>enabled: {String(enabled)}</div>
                <div>template: {String(trigger?.templateId || '')}</div>
              </div>
              {canManagePolicies ? (
                <div className="btn-row">
                  <button type="button" className="btn-sm btn-secondary" onClick={() => {
                    setEditingTriggerId(triggerId);
                    setTriggerForm({
                      name: String(trigger?.name || ''),
                      type: String(trigger?.type || 'webhook'),
                      templateId: String(trigger?.templateId || String(templates[0]?.id || '')),
                      provider: String(config?.provider || ''),
                      hostIds: Array.isArray(config?.hostIds) ? config.hostIds.join(', ') : '',
                      hostLabels: Array.isArray(config?.hostLabels) ? config.hostLabels.join(', ') : '',
                      maxConcurrency: config?.maxConcurrency != null ? String(config.maxConcurrency) : '',
                      timezone: String(config?.timezone || 'UTC'),
                      webhookSecret: '',
                      githubCommand: String(config?.githubCommand || ''),
                      githubLabel: String(config?.githubLabel || ''),
                      githubRepository: String(config?.githubRepository || ''),
                      cron: String(config?.cron || ''),
                      inputs: renderInputsText(config?.inputs || {}),
                      policyApprove: !!config?.policyApprove,
                    });
                  }}>Edit</button>
                  <button type="button" className="btn-sm btn-secondary" onClick={() => toggleTriggerMutation.mutate({ id: triggerId, enabled: !enabled })}>{enabled ? 'Disable' : 'Enable'}</button>
                  <button type="button" className="btn-sm btn-danger" onClick={() => { if (window.confirm('Delete execution trigger?')) deleteTriggerMutation.mutate(triggerId); }}>Delete</button>
                </div>
              ) : null}
            </div>
          );
        }) : <div className="card">No execution triggers configured.</div>}
      </div>
    </div>
  );

  return (
    <section id="view-profiles" className="view">
      <div className="section-head">
        <h2 id="profiles-title">{mode === 'providers' ? 'Providers' : 'Policies'}</h2>
        <div className="section-actions">
          <button id="profiles-refresh" className="btn-sm btn-secondary" onClick={() => void refreshAll()}>Refresh</button>
        </div>
      </div>
      {mode === 'providers' ? renderProviders() : renderPolicies()}
    </section>
  );
}
