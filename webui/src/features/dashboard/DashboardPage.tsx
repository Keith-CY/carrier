import { formatDateTime } from '../../lib/format';
import { ExecutionDetailContent, executionAttributionParts } from '../executions/ExecutionDetailContent';
import { useDashboardData, useDashboardExecutionDetail } from './useDashboardData';

function statusIcon(value: unknown): string {
  const status = String(value || '').trim().toLowerCase();
  if (status === 'running' || status === 'healthy') return '🟢';
  if (status === 'error') return '🔴';
  return '⚪';
}

function DashboardExecutionDetail({ executionId }: { executionId: string }) {
  const executionDetailQuery = useDashboardExecutionDetail(executionId);

  if (executionDetailQuery.isLoading) return <div className="text-dim">Loading details…</div>;
  if (executionDetailQuery.isError) return <div className="text-dim">Load failed: {(executionDetailQuery.error as Error).message}</div>;
  const payload = executionDetailQuery.data || {};
  return (
    <ExecutionDetailContent
      execution={payload.execution || {}}
      workers={Array.isArray(payload.workers) ? payload.workers : []}
      onDownloadArtifact={async () => undefined}
    />
  );
}

export function DashboardPage() {
  const {
    featureFlags,
    authz,
    instancesQuery,
    agentCatalogQuery,
    addAgentModalOpen,
    setAddAgentModalOpen,
    agentCatalog,
    instances,
    runningInstances,
    recentExecutions,
    activeRecentExecutions,
    expandedExecutionIds,
    quickLaunchMessage,
    quickLaunchDraft,
    quickLaunchAdvancedVisible,
    quickLaunchPlan,
    previewMutation,
    runMutation,
    providerOptions,
    templates,
    selectedTemplate,
    hostOptions,
    setQuickLaunchMode,
    setQuickLaunchGoal,
    setQuickLaunchProvider,
    setQuickLaunchMaxConcurrency,
    setQuickLaunchHostLabels,
    setQuickLaunchTemplateId,
    setQuickLaunchTemplateInput,
    toggleQuickLaunchHost,
    resetQuickLaunch,
    setQuickLaunchAdvancedVisible,
    clearQuickLaunchPreview,
    previewQuickLaunch,
    runQuickLaunch,
    refreshExecutions,
    refreshInstances,
    handleInstanceAction,
    toggleExecutionExpansion,
    executionCounts,
    navigate,
  } = useDashboardData();

  return (
    <section id="view-dashboard" className="view">
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
          <label htmlFor="quick-launch-mode">Mode</label>
          <select id="quick-launch-mode" value={quickLaunchDraft.mode} onChange={(event) => setQuickLaunchMode(event.target.value === 'template' ? 'template' : 'goal')}>
            <option value="goal">Goal</option>
            <option value="template">Template</option>
          </select>
        </div>
        <div id="quick-launch-goal-field" className={quickLaunchDraft.mode === 'goal' ? '' : 'hidden'}>
          <label htmlFor="quick-launch-goal">Goal</label>
          <textarea
            id="quick-launch-goal"
            rows={4}
            placeholder="Describe the task for the base agent to decompose and dispatch."
            value={quickLaunchDraft.goal}
            onChange={(event) => setQuickLaunchGoal(event.target.value)}
          />
        </div>
        <div id="quick-launch-template-field" className={quickLaunchDraft.mode === 'template' ? '' : 'hidden'}>
          <label htmlFor="quick-launch-template">Template</label>
          <select id="quick-launch-template" value={quickLaunchDraft.templateId} onChange={(event) => setQuickLaunchTemplateId(event.target.value)}>
            <option value="">Select a template</option>
            {templates.map((item: any) => (
              <option key={String(item?.id || '')} value={String(item?.id || '')}>
                {String(item?.name || item?.id || '')}
              </option>
            ))}
          </select>
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
                  ? `Approval: ${String(quickLaunchPlan?.approvalScope || 'infrastructure_only')} · ${String(quickLaunchPlan?.templateId || '').trim() ? `Template: ${String(quickLaunchPlan.templateId).trim()} · ` : ''}Task units: ${Array.isArray(quickLaunchPlan?.taskUnits) ? quickLaunchPlan.taskUnits.length : 0} · Max concurrency: ${Number(quickLaunchPlan?.maxConcurrency || 0)}`
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

      <div className="section-head">
        <h2>Installed Agents</h2>
        <div className="section-actions">
          <button id="dashboard-add-agent" type="button" onClick={() => setAddAgentModalOpen(true)}>Add Agent</button>
          <button id="refresh-instances" className="btn-sm btn-secondary" onClick={() => void refreshInstances()}>
            Refresh
          </button>
        </div>
      </div>
      <p id="instance-summary" className="text-dim">
        {instancesQuery.isLoading ? 'Loading…' : instances.length ? `Total: ${instances.length} · Running: ${runningInstances}` : 'Use Add Agent to install and configure a new instance.'}
      </p>
      <div id="instance-list" className="agent-grid">
        {!instancesQuery.isLoading && !instances.length ? 'No installed agent instances.' : instances.map((item: any) => {
          const instanceId = String(item?.id || item?.ID || '').trim();
          const agentId = String(item?.agent_id || item?.agentID || item?.agent || item?.type || 'unknown').trim();
          const runtime = String(item?.runtime_state || item?.runtimeState || item?.runtime || 'unknown').trim();
          const pairRequired = !!(item?.pair_required || item?.pairRequired);
          const pairedChatId = String(item?.paired_chat_id || item?.pairedChatId || '').trim();
          let metaText = `Type: ${agentId} · Channel: ${String(item?.channel || 'n/a')} · Provider: ${String(item?.provider || 'n/a')}`;
          if (pairRequired) metaText += ' · Pair: required';
          else if (pairedChatId) metaText += ` · Paired chat: ${pairedChatId}`;
          return (
            <div key={instanceId} className="agent-card">
              <h4>{instanceId}</h4>
              <div className="agent-status">{statusIcon(runtime)} {runtime}</div>
              <div className="instance-meta">{metaText}</div>
              <div className="btn-row">
                <button className="btn-sm" onClick={() => void handleInstanceAction(instanceId, 'start')}>Start</button>
                <button className="btn-sm btn-secondary" onClick={() => void handleInstanceAction(instanceId, 'stop')}>Stop</button>
                <button className="btn-sm btn-danger" onClick={() => void handleInstanceAction(instanceId, 'uninstall')}>Uninstall</button>
              </div>
            </div>
          );
        })}
      </div>

      <div id="dashboard-executions-section" className={`dashboard-stack${featureFlags.remoteControlPlaneEnabled && authz.permissions.viewExecutions ? '' : ' hidden'}`}>
        <div className="section-head">
          <h2>Recent Executions</h2>
          <div className="section-actions">
            <button id="refresh-executions" className="btn-sm btn-secondary" onClick={() => void refreshExecutions()}>
              Refresh
            </button>
          </div>
        </div>
        <p id="execution-summary" className="text-dim">
          {recentExecutions.length ? `Recent: ${recentExecutions.length} · Active: ${activeRecentExecutions}` : 'No executions recorded yet.'}
        </p>
        <div id="execution-list" className="agent-grid">
          {recentExecutions.map((execution: any) => {
            const executionId = String(execution?.id || '').trim();
            const statusText = String(execution?.status || 'unknown').trim();
            const counts = executionCounts(execution);
            const isExpanded = expandedExecutionIds.has(executionId);
            return (
              <div key={executionId} className="agent-card execution-card">
                <div className="section-head">
                  <h4>{executionId || 'execution'}</h4>
                  <span className={statusText === 'completed' ? 'badge badge-ok' : statusText === 'running' ? 'badge badge-warn' : 'badge badge-unknown'}>{statusText || 'unknown'}</span>
                </div>
                <div className="execution-goal">{String(execution?.goal || '').trim() || '(no goal)'}</div>
                <div className="instance-meta">
                  Tasks: {counts.taskUnits.length} · Completed: {counts.completed} · Failed: {counts.failed} · Updated: {formatDateTime(execution?.updatedAt)}
                </div>
                {executionAttributionParts(execution).length ? <div className="instance-meta">{executionAttributionParts(execution).join(' · ')}</div> : null}
                <div className="btn-row">
                  <button className="btn-sm" onClick={() => navigate(`/executions/${encodeURIComponent(executionId)}`)}>Open</button>
                  <button className="btn-sm btn-secondary" onClick={() => toggleExecutionExpansion(executionId)}>
                    {isExpanded ? 'Hide Details' : 'View Details'}
                  </button>
                </div>
                <div className={`execution-details${isExpanded ? '' : ' hidden'}`}>
                  {isExpanded ? <DashboardExecutionDetail executionId={executionId} /> : null}
                </div>
              </div>
            );
          })}
        </div>
      </div>

      <div id="add-agent-overlay" className={`overlay${addAgentModalOpen ? '' : ' hidden'}`}>
        <div className="card install-modal">
          <div className="section-head">
            <h3>Add Agent</h3>
          </div>
          <p className="text-dim">Select an agent to start the add flow.</p>
          <ul id="add-agent-options" className="agent-select-list">
            {agentCatalog.map((item: any) => {
              const id = String(item?.id || '').trim();
              return (
                <li key={id}>
                  <button
                    type="button"
                    className="agent-select-item"
                    onClick={() => {
                      setAddAgentModalOpen(false);
                      navigate(`/add/${encodeURIComponent(id)}`);
                    }}
                  >
                    {id}
                  </button>
                </li>
              );
            })}
          </ul>
          <div id="add-agent-msg">
            {agentCatalogQuery.isLoading ? <p className="msg-info">Loading agents…</p> : null}
            {agentCatalogQuery.isError ? <p className="msg-error">{`Error loading agents: ${(agentCatalogQuery.error as Error).message}`}</p> : null}
            {!agentCatalogQuery.isLoading && !agentCatalogQuery.isError && !agentCatalog.length ? <p className="msg-error">No agents available.</p> : null}
          </div>
          <div className="btn-row">
            <button id="add-agent-cancel" className="btn-secondary" type="button" onClick={() => setAddAgentModalOpen(false)}>
              Cancel
            </button>
          </div>
        </div>
      </div>
    </section>
  );
}
