import { CUSTOM_GOAL_PRESET_ID } from '../model';
import type { DashboardData } from '../useDashboardData';

export function QuickLaunchCard({ data }: { data: DashboardData }) {
  const {
    featureFlags,
    authz,
    quickLaunchMessage,
    quickLaunchDraft,
    quickLaunchAdvancedVisible,
    quickLaunchPlan,
    previewMutation,
    runMutation,
    templates,
    selectedTemplate,
    providerOptions,
    hostOptions,
    selectQuickLaunchPreset,
    setQuickLaunchGoal,
    setQuickLaunchProvider,
    setQuickLaunchMaxConcurrency,
    setQuickLaunchHostLabels,
    setQuickLaunchTemplateInput,
    toggleQuickLaunchHost,
    resetQuickLaunch,
    setQuickLaunchAdvancedVisible,
    clearQuickLaunchPreview,
    previewQuickLaunch,
    runQuickLaunch,
  } = data;
  const customGoalSelected = quickLaunchDraft.selectedPresetId === CUSTOM_GOAL_PRESET_ID;

  return (
    <div id="dashboard-quick-launch-section" className={`card dashboard-stack${featureFlags.remoteControlPlaneEnabled && authz.permissions.launchExecutions ? '' : ' hidden'}`}>
      <div className="section-head">
        <div>
          <h2>Quick Launch</h2>
          <p className="text-dim">Preview the plan first, then create and authorize the execution.</p>
        </div>
        <div className="section-actions">
          <button id="quick-launch-preview" className="btn-sm" onClick={previewQuickLaunch} disabled={previewMutation.isPending}>
            Preview Plan
          </button>
          <button id="quick-launch-reset" className="btn-sm btn-secondary" type="button" onClick={resetQuickLaunch}>
            Reset
          </button>
          <button
            id="quick-launch-advanced-toggle"
            className="btn-sm btn-secondary"
            type="button"
            onClick={() => setQuickLaunchAdvancedVisible((value) => !value)}
          >
            {quickLaunchAdvancedVisible ? 'Hide Advanced' : 'Advanced'}
          </button>
        </div>
      </div>
      <div>
        <label>Preset</label>
        <div id="quick-launch-presets" className="quick-launch-hosts">
          {templates.map((item: any) => {
            const presetId = String(item?.id || '').trim();
            const selected = presetId !== '' && quickLaunchDraft.selectedPresetId === presetId;
            return (
              <button
                key={presetId}
                id={`quick-launch-preset-${presetId}`}
                className={`btn-sm${selected ? '' : ' btn-secondary'}`}
                type="button"
                onClick={() => selectQuickLaunchPreset(presetId, item)}
              >
                {String(item?.name || presetId)}
              </button>
            );
          })}
          <button
            id="quick-launch-preset-custom-goal"
            className={`btn-sm${customGoalSelected ? '' : ' btn-secondary'}`}
            type="button"
            onClick={() => selectQuickLaunchPreset(CUSTOM_GOAL_PRESET_ID)}
          >
            Custom Goal
          </button>
        </div>
      </div>
      <div id="quick-launch-goal-field" className={customGoalSelected ? '' : 'hidden'}>
        <label htmlFor="quick-launch-goal">Goal</label>
        <textarea
          id="quick-launch-goal"
          rows={4}
          placeholder="Describe the task for the base agent to decompose and dispatch."
          value={quickLaunchDraft.goal}
          onChange={(event) => setQuickLaunchGoal(event.target.value)}
        />
      </div>
      <div id="quick-launch-template-field" className={customGoalSelected ? 'hidden' : ''}>
        {selectedTemplate ? (
          <>
            <div className="section-head">
              <div>
                <h3>{String(selectedTemplate?.name || selectedTemplate?.id || '')}</h3>
                <p className="text-dim">{String(selectedTemplate?.description || 'Repeatable launch preset for this execution flow.')}</p>
              </div>
            </div>
            <p className="text-dim">
              {`Defaults · Approval: ${String(selectedTemplate?.defaultLaunchConfig?.approvalScope || 'infrastructure_only')} · Max concurrency: ${Number(selectedTemplate?.defaultLaunchConfig?.maxConcurrency || 0) || 'auto'}${Array.isArray(selectedTemplate?.defaultLaunchConfig?.hostLabels) && selectedTemplate.defaultLaunchConfig.hostLabels.length ? ` · Host labels: ${selectedTemplate.defaultLaunchConfig.hostLabels.join(', ')}` : ''}`}
            </p>
          </>
        ) : null}
        <div id="quick-launch-template-inputs" className="form-grid" style={{ marginTop: '12px' }}>
          {(Array.isArray(selectedTemplate?.inputSchema) ? selectedTemplate.inputSchema : []).map((field: any) => {
            const key = String(field?.id || '').trim();
            return (
              <div key={key}>
                <label htmlFor={`quick-launch-template-input-${key}`}>{String(field?.label || key)}</label>
                <input
                  id={`quick-launch-template-input-${key}`}
                  data-quick-launch-template-input={key}
                  type="text"
                  placeholder={String(field?.placeholder || '')}
                  title={String(field?.description || '')}
                  value={quickLaunchDraft.templateInputs[key] ?? String(field?.defaultValue || '')}
                  onChange={(event) => setQuickLaunchTemplateInput(key, event.target.value)}
                />
              </div>
            );
          })}
        </div>
      </div>
      <div id="quick-launch-advanced" className={`quick-launch-advanced${quickLaunchAdvancedVisible ? '' : ' hidden'}`}>
        <div className="form-grid">
          <div>
            <label htmlFor="quick-launch-provider">Provider</label>
            <select id="quick-launch-provider" value={quickLaunchDraft.provider} onChange={(event) => setQuickLaunchProvider(event.target.value)}>
              <option value="">System default</option>
              {providerOptions.map((provider: any) => (
                <option key={String(provider?.id || '')} value={String(provider?.id || '')}>
                  {String(provider?.name || provider?.id || '')}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label htmlFor="quick-launch-max-concurrency">Max Concurrency</label>
            <input
              id="quick-launch-max-concurrency"
              type="number"
              min={1}
              max={64}
              placeholder="Auto"
              value={quickLaunchDraft.maxConcurrency}
              onChange={(event) => setQuickLaunchMaxConcurrency(event.target.value)}
            />
          </div>
        </div>
        <div>
          <label htmlFor="quick-launch-host-labels">Host Labels</label>
          <input
            id="quick-launch-host-labels"
            type="text"
            placeholder="gpu, prod"
            value={quickLaunchDraft.hostLabels}
            onChange={(event) => setQuickLaunchHostLabels(event.target.value)}
          />
          <p className="text-dim">Optional selector. When set, the preview targets matching labeled hosts instead of explicit host checkboxes.</p>
        </div>
        <div>
          <label>Hosts</label>
          <div id="quick-launch-hosts" className="quick-launch-hosts">
            {hostOptions.map((item: any) => {
              const hostId = String(item?.id || '').trim();
              return (
                <label key={hostId} className="quick-launch-host-option">
                  <input
                    type="checkbox"
                    value={hostId}
                    checked={quickLaunchDraft.selectedHosts.includes(hostId)}
                    onChange={() => toggleQuickLaunchHost(hostId)}
                  />
                  <span>{String(item?.name || hostId)}</span>
                </label>
              );
            })}
          </div>
        </div>
      </div>
      <div id="quick-launch-msg">{quickLaunchMessage.text ? <p className={`msg-${quickLaunchMessage.type}`}>{quickLaunchMessage.text}</p> : null}</div>
      <div id="quick-launch-preview-card" className={`quick-launch-preview${quickLaunchPlan ? '' : ' hidden'}`}>
        <div className="section-head">
          <div>
            <h3>Plan Preview</h3>
            <p id="quick-launch-preview-summary" className="text-dim">
              {quickLaunchPlan
                ? `Approval: ${String(quickLaunchPlan?.approvalScope || 'infrastructure_only')} · ${String(quickLaunchPlan?.templateId || '').trim() ? `Template: ${String(quickLaunchPlan.templateId).trim()}${String(quickLaunchPlan?.templateVersion || '').trim() ? ` (${String(quickLaunchPlan.templateVersion).trim()})` : ''} · ` : 'Template: Custom Goal · '}Task units: ${Array.isArray(quickLaunchPlan?.taskUnits) ? quickLaunchPlan.taskUnits.length : 0} · Max concurrency: ${Number(quickLaunchPlan?.maxConcurrency || 0)}`
                : ''}
            </p>
          </div>
          <div className="section-actions">
            <button id="quick-launch-edit" className="btn-sm btn-secondary" type="button" onClick={clearQuickLaunchPreview}>
              Back to Edit
            </button>
            <button id="quick-launch-run" className="btn-sm" type="button" onClick={runQuickLaunch} disabled={runMutation.isPending}>
              Run
            </button>
          </div>
        </div>
        <div className="quick-launch-preview-grid">
          <div className="quick-launch-preview-block">
            <h4>Planner Tasks</h4>
            <div id="quick-launch-preview-tasks">
              {(Array.isArray(quickLaunchPlan?.plannerTasks) ? quickLaunchPlan.plannerTasks : []).map((task: any) => (
                <div key={String(task?.id || '')} className="quick-launch-line">
                  {String(task?.id || 'task')} · {String(task?.agentId || 'zeroclaw')} · {String(task?.input || '').trim()}
                </div>
              ))}
            </div>
          </div>
          <div className="quick-launch-preview-block">
            <h4>Workers</h4>
            <div id="quick-launch-preview-workers">
              {(Array.isArray(quickLaunchPlan?.requiredWorkers) ? quickLaunchPlan.requiredWorkers : []).map((worker: any, index: number) => {
                const hostLabels = Array.isArray(worker?.hostLabels) ? worker.hostLabels.map((value: unknown) => String(value || '').trim()).filter(Boolean) : [];
                const hostTarget = String(worker?.hostId || '').trim() || (hostLabels.length ? `labels[${hostLabels.join(',')}]` : 'local');
                return (
                  <div key={`${hostTarget}-${worker?.agentId || 'zeroclaw'}-${index}`} className="quick-launch-line">
                    {hostTarget}/{String(worker?.agentId || 'zeroclaw')} · count={String(worker?.count || 1)}
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
