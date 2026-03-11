import type { ReactNode } from 'react';
import { formatHostMeta, formatOperationMeta, updateEditorFromHost } from './model';
import { useHostsData } from './useHostsData';

function renderMessage(message: { type: 'info' | 'success' | 'error'; text: string }): ReactNode {
  if (!message.text) return null;
  return <p className={`msg-${message.type}`}>{message.text}</p>;
}

export function HostsPage() {
  const {
    featuresLoading,
    canManageHosts,
    hosts,
    sshAliases,
    serversMessage,
    setServersMessage,
    manageMessage,
    editor,
    setEditor,
    editingHostId,
    setEditingHostId,
    selectedHostId,
    selectedHost,
    manageForm,
    setManageForm,
    manageBusy,
    editorBusy,
    hostOps,
    opMeta,
    instancesText,
    instanceStatusText,
    logsText,
    configText,
    setConfigText,
    sessionsText,
    memoryText,
    refresh,
    resetEditor,
    showManageHost,
    handleSaveHost,
    handleDeleteHost,
    handleCheckHost,
    loadManageInstances,
    loadManageInstanceStatus,
    installManageInstance,
    repairManageInstance,
    loadManageLogs,
    syncManageInstance,
    loadManageSyncStatus,
    diagnoseManageInstance,
    reconcileManageInstance,
    rollbackManageInstance,
    installManageCodeAgent,
    healthManageCodeAgent,
    versionManageCodeAgent,
    runManageCodeAgent,
    loadManageConfig,
    applyManageConfig,
    loadManageSessions,
    applySessionAction,
    loadManageMemory,
  } = useHostsData();

  if (featuresLoading) {
    return <section id="view-servers" className="view view-servers-surface"><div className="card">Loading hosts…</div></section>;
  }

  return (
    <section id="view-servers" className="view view-servers-surface">
      <div className="section-head">
        <h2>Hosts</h2>
        <div className="section-actions">
          <button id="servers-refresh" className="btn-sm btn-secondary" onClick={() => void refresh()}>
            Refresh
          </button>
        </div>
      </div>

      <div className="card">
        <h3>Add / Update Server</h3>
        <div className="form-grid">
          <div>
            <label htmlFor="server-name">Name</label>
            <input id="server-name" type="text" placeholder="prod-eu-1" value={editor.name} onChange={(event) => setEditor((current) => ({ ...current, name: event.target.value }))} />
          </div>
          <div>
            <label htmlFor="server-auth-mode">Auth Mode</label>
            <select id="server-auth-mode" value={editor.authMode} onChange={(event) => setEditor((current) => ({ ...current, authMode: event.target.value }))}>
              <option value="private_key">private_key</option>
              <option value="ssh_config">ssh_config</option>
            </select>
          </div>
          <div>
            <label htmlFor="server-host">Host</label>
            <input id="server-host" type="text" placeholder="192.168.1.10" value={editor.host} onChange={(event) => setEditor((current) => ({ ...current, host: event.target.value }))} />
          </div>
          <div>
            <label htmlFor="server-port">Port</label>
            <input id="server-port" type="text" placeholder="22" value={editor.port} onChange={(event) => setEditor((current) => ({ ...current, port: event.target.value }))} />
          </div>
          <div>
            <label htmlFor="server-user">User</label>
            <input id="server-user" type="text" placeholder="ubuntu" value={editor.user} onChange={(event) => setEditor((current) => ({ ...current, user: event.target.value }))} />
          </div>
          <div>
            <label htmlFor="server-runtime-mode">Runtime Mode</label>
            <select id="server-runtime-mode" value={editor.runtimeMode} onChange={(event) => setEditor((current) => ({ ...current, runtimeMode: event.target.value }))}>
              <option value="on_demand">on_demand</option>
              <option value="managed_gateway">managed_gateway</option>
            </select>
          </div>
          <div>
            <label htmlFor="server-labels">Labels</label>
            <input id="server-labels" type="text" placeholder="prod, gpu" value={editor.labels} onChange={(event) => setEditor((current) => ({ ...current, labels: event.target.value }))} />
          </div>
          <div>
            <label htmlFor="server-key-path">Key Path</label>
            <input id="server-key-path" type="text" placeholder="~/.ssh/id_ed25519" value={editor.keyPath} disabled={editor.authMode !== 'private_key'} onChange={(event) => setEditor((current) => ({ ...current, keyPath: event.target.value }))} />
          </div>
          <div>
            <label htmlFor="server-ssh-config-host">SSH Config Host</label>
            <input id="server-ssh-config-host" type="text" list="server-ssh-config-host-options" placeholder="my-prod-host" value={editor.sshConfigHost} disabled={editor.authMode === 'private_key'} onChange={(event) => setEditor((current) => ({ ...current, sshConfigHost: event.target.value }))} />
            <datalist id="server-ssh-config-host-options">
              {sshAliases.map((alias) => <option key={alias} value={alias} />)}
            </datalist>
            <select
              id="server-ssh-config-host-select"
              className={editor.authMode === 'private_key' || sshAliases.length === 0 ? 'hidden' : ''}
              style={{ marginTop: '8px' }}
              disabled={editor.authMode === 'private_key' || sshAliases.length === 0}
              value=""
              onChange={(event) => {
                if (!event.target.value) return;
                setEditor((current) => ({ ...current, sshConfigHost: event.target.value }));
                event.target.value = '';
              }}
            >
              <option value="">{sshAliases.length ? 'Select detected SSH alias…' : 'No detected SSH aliases'}</option>
              {sshAliases.map((alias) => <option key={alias} value={alias}>{alias}</option>)}
            </select>
            <p id="server-ssh-config-host-hint" className={`text-dim${editor.authMode === 'private_key' ? ' hidden' : ''}`}>
              {sshAliases.length ? `Detected ${sshAliases.length} alias(es) from local SSH config. Select from dropdown or type manually.` : 'No aliases detected from local SSH config. You can still type one manually.'}
            </p>
          </div>
        </div>
        <div className="btn-row">
          <button id="server-save" disabled={!canManageHosts || editorBusy} onClick={() => void handleSaveHost()}>{editingHostId ? 'Update Host' : 'Save Host'}</button>
          <button id="server-cancel-edit" className={`btn-sm btn-secondary${editingHostId ? '' : ' hidden'}`} disabled={!canManageHosts || editorBusy} onClick={() => { resetEditor(true); setServersMessage({ type: 'info', text: 'Host edit cancelled.' }); }}>
            Cancel Edit
          </button>
        </div>
        <p id="server-editor-state" className="text-dim">{editingHostId ? `Editing host: ${editingHostId}` : ''}</p>
        <div id="servers-msg">{renderMessage(serversMessage)}</div>
      </div>

      <div id="servers-list" className="agent-grid server-host-list">
        {hosts.length === 0 ? (
          <div className="card">No remote servers configured.</div>
        ) : hosts.map((host) => {
          const hostId = String(host?.id || '');
          return (
            <div key={hostId} className="agent-card">
              <h4>{String(host?.name || hostId)}</h4>
              <div className="instance-meta" style={{ whiteSpace: 'pre-line' }}>{formatHostMeta(host, hostOps[hostId])}</div>
              {canManageHosts ? (
                <div className="btn-row">
                  <button className="btn-sm btn-secondary" onClick={() => void handleCheckHost(hostId)}>Check</button>
                  <button className="btn-sm" onClick={() => showManageHost(hostId)}>Manage</button>
                  <button className="btn-sm btn-secondary" onClick={() => { setEditingHostId(hostId); setEditor(updateEditorFromHost(host)); setServersMessage({ type: 'info', text: `Editing remote host: ${hostId}` }); }}>Edit</button>
                  <button className="btn-sm btn-danger" onClick={() => void handleDeleteHost(hostId)}>Delete</button>
                </div>
              ) : null}
            </div>
          );
        })}
      </div>

      <div id="server-manage-card" className={`card servers-manage-panel${selectedHost ? '' : ' hidden'}`}>
        <h3>Server Runtime Data</h3>
        <p id="server-manage-host-label" className="text-dim">{selectedHost ? `Selected host: ${String(selectedHost?.name || selectedHost?.id || selectedHostId)} (${selectedHostId})` : ''}</p>
        <div className="form-grid">
          <div>
            <label htmlFor="server-manage-agent-id">Agent ID</label>
            <input id="server-manage-agent-id" type="text" placeholder="main" value={manageForm.agentId} onChange={(event) => setManageForm((current) => ({ ...current, agentId: event.target.value }))} />
          </div>
          <div>
            <label htmlFor="server-manage-session-id">Session ID</label>
            <input id="server-manage-session-id" type="text" placeholder="session id" value={manageForm.sessionId} onChange={(event) => setManageForm((current) => ({ ...current, sessionId: event.target.value }))} />
          </div>
          <div>
            <label htmlFor="server-manage-log-tail">Log Tail</label>
            <input id="server-manage-log-tail" type="text" placeholder="200" value={manageForm.logTail} onChange={(event) => setManageForm((current) => ({ ...current, logTail: event.target.value }))} />
          </div>
          <div>
            <label htmlFor="server-manage-sync-mode">Sync Mode</label>
            <select id="server-manage-sync-mode" value={manageForm.syncMode} onChange={(event) => setManageForm((current) => ({ ...current, syncMode: event.target.value }))}>
              <option value="pull_validate_push">pull_validate_push</option>
              <option value="always_push">always_push</option>
              <option value="manual">manual</option>
            </select>
          </div>
          <div>
            <label htmlFor="server-manage-rollback-commit">Rollback Commit <span className="text-dim">(optional)</span></label>
            <input id="server-manage-rollback-commit" type="text" placeholder="commit hash" value={manageForm.rollbackCommit} onChange={(event) => setManageForm((current) => ({ ...current, rollbackCommit: event.target.value }))} />
          </div>
          <div>
            <label htmlFor="server-manage-codeagent-backend">CodeAgent Backend</label>
            <select id="server-manage-codeagent-backend" value={manageForm.codeagentBackend} onChange={(event) => setManageForm((current) => ({ ...current, codeagentBackend: event.target.value }))}>
              <option value="codex">codex</option>
              <option value="opencode">opencode</option>
            </select>
          </div>
          <div>
            <label htmlFor="server-manage-codeagent-workspace-root">CodeAgent Workspace Root</label>
            <input id="server-manage-codeagent-workspace-root" type="text" value={manageForm.codeagentWorkspaceRoot} placeholder="/workspace" onChange={(event) => setManageForm((current) => ({ ...current, codeagentWorkspaceRoot: event.target.value }))} />
          </div>
          <div>
            <label htmlFor="server-manage-codeagent-capability">CodeAgent Capability</label>
            <select id="server-manage-codeagent-capability" value={manageForm.codeagentCapability} onChange={(event) => setManageForm((current) => ({ ...current, codeagentCapability: event.target.value }))}>
              <option value="run_shell">run_shell</option>
              <option value="read_file">read_file</option>
              <option value="write_file">write_file</option>
              <option value="apply_patch">apply_patch</option>
              <option value="run_shell_redirect">run_shell_redirect</option>
            </select>
          </div>
          <div>
            <label htmlFor="server-manage-codeagent-write-mode">CodeAgent Write Mode</label>
            <select id="server-manage-codeagent-write-mode" value={manageForm.codeagentWriteMode} onChange={(event) => setManageForm((current) => ({ ...current, codeagentWriteMode: event.target.value }))}>
              <option value="overwrite">overwrite</option>
              <option value="append">append</option>
            </select>
          </div>
          <div style={{ gridColumn: '1 / -1' }}>
            <label htmlFor="server-manage-codeagent-command">CodeAgent Command <span className="text-dim">(for run_shell)</span></label>
            <input id="server-manage-codeagent-command" type="text" placeholder="echo hello from remote" value={manageForm.codeagentCommand} onChange={(event) => setManageForm((current) => ({ ...current, codeagentCommand: event.target.value }))} />
          </div>
          <div style={{ gridColumn: '1 / -1' }}>
            <label htmlFor="server-manage-codeagent-path">CodeAgent Path <span className="text-dim">(for file capabilities)</span></label>
            <input id="server-manage-codeagent-path" type="text" placeholder="README.md" value={manageForm.codeagentPath} onChange={(event) => setManageForm((current) => ({ ...current, codeagentPath: event.target.value }))} />
          </div>
          <div style={{ gridColumn: '1 / -1' }}>
            <label htmlFor="server-manage-codeagent-content">CodeAgent Content <span className="text-dim">(for write/apply_patch)</span></label>
            <textarea id="server-manage-codeagent-content" rows={4} placeholder="content or patch payload" value={manageForm.codeagentContent} onChange={(event) => setManageForm((current) => ({ ...current, codeagentContent: event.target.value }))} />
          </div>
        </div>
        <div className="btn-row">
          <button id="server-manage-load-instances" className="btn-sm btn-secondary" disabled={manageBusy} onClick={() => void loadManageInstances()}>Load Instances</button>
          <button id="server-manage-instance-status" className="btn-sm btn-secondary" disabled={manageBusy} onClick={() => void loadManageInstanceStatus('Instance Status')}>Instance Status</button>
          <button id="server-manage-install-instance" className="btn-sm" disabled={manageBusy} onClick={() => void installManageInstance()}>Install (Live)</button>
          <button id="server-manage-repair-instance" className="btn-sm btn-secondary" disabled={manageBusy} onClick={() => void repairManageInstance()}>Repair</button>
          <button id="server-manage-load-logs" className="btn-sm btn-secondary" disabled={manageBusy} onClick={() => void loadManageLogs()}>Load Logs</button>
        </div>
        <div className="btn-row" style={{ marginTop: '8px' }}>
          <button id="server-manage-sync-instance" className="btn-sm btn-secondary" disabled={manageBusy} onClick={() => void syncManageInstance()}>Sync</button>
          <button id="server-manage-sync-status" className="btn-sm btn-secondary" disabled={manageBusy} onClick={() => void loadManageSyncStatus()}>Sync Status</button>
          <button id="server-manage-diagnose-instance" className="btn-sm btn-secondary" disabled={manageBusy} onClick={() => void diagnoseManageInstance()}>Diagnose Drift</button>
          <button id="server-manage-reconcile-instance" className="btn-sm btn-secondary" disabled={manageBusy} onClick={() => void reconcileManageInstance()}>Reconcile</button>
          <button id="server-manage-rollback-instance" className="btn-sm btn-danger" disabled={manageBusy} onClick={() => void rollbackManageInstance()}>Rollback</button>
        </div>
        <div className="btn-row" style={{ marginTop: '8px' }}>
          <button id="server-manage-codeagent-install" className="btn-sm" disabled={manageBusy} onClick={() => void installManageCodeAgent()}>CodeAgent Install</button>
          <button id="server-manage-codeagent-health" className="btn-sm btn-secondary" disabled={manageBusy} onClick={() => void healthManageCodeAgent()}>CodeAgent Health</button>
          <button id="server-manage-codeagent-version" className="btn-sm btn-secondary" disabled={manageBusy} onClick={() => void versionManageCodeAgent()}>CodeAgent Version</button>
          <button id="server-manage-codeagent-run" className="btn-sm btn-secondary" disabled={manageBusy} onClick={() => void runManageCodeAgent()}>CodeAgent Run</button>
        </div>
        <pre id="server-manage-instances" className="code-block text-dim" style={{ marginTop: '8px', minHeight: '72px' }}>{instancesText}</pre>
        <div id="server-manage-instance-status-out" className="manage-output" style={{ marginTop: '8px', minHeight: '72px', whiteSpace: 'pre-line' }}>{instanceStatusText}</div>
        <div id="server-manage-logs" className="manage-output" style={{ marginTop: '8px', minHeight: '72px', whiteSpace: 'pre-line' }}>{logsText}</div>
        <p id="server-manage-stream-status" className="text-dim" />
        <div id="server-manage-diagnosis" className="manage-output" style={{ marginTop: '8px' }} />

        <div className="card" style={{ marginTop: '10px' }}>
          <h4>SSG Agent Chat</h4>
          <div id="server-manage-chat-messages" className="chat-messages manage-chat-messages" />
          <div className="chat-input-row">
            <input id="server-manage-chat-input" type="text" placeholder="Message selected remote agent…" autoComplete="off" disabled />
            <button id="server-manage-chat-send" className="btn-sm" disabled>Send</button>
          </div>
          <div className="btn-row" style={{ marginTop: '8px' }}>
            <button id="server-manage-chat-reset-session" className="btn-sm btn-secondary" disabled>New Session</button>
            <button id="server-manage-chat-cancel" className="btn-sm btn-secondary" disabled>Cancel Stream</button>
            <button id="server-manage-chat-retry" className="btn-sm btn-secondary" disabled>Retry Last</button>
          </div>
          <p id="server-manage-chat-status" className="text-dim">Remote chat migration pending.</p>
        </div>

        <div className="btn-row">
          <button id="server-manage-load-config" className="btn-sm btn-secondary" disabled={manageBusy} onClick={() => void loadManageConfig()}>Load Config</button>
          <button id="server-manage-apply-config" className="btn-sm" disabled={manageBusy} onClick={() => void applyManageConfig()}>Apply Config Patch</button>
        </div>
        <textarea id="server-manage-config" rows={10} style={{ marginTop: '8px' }} placeholder='{"agents":{"defaults":{"model":"gpt-5"}}}' value={configText} onChange={(event) => setConfigText(event.target.value)} />

        <div className="btn-row" style={{ marginTop: '10px' }}>
          <button id="server-manage-load-sessions" className="btn-sm btn-secondary" disabled={manageBusy} onClick={() => void loadManageSessions()}>Load Sessions</button>
          <button id="server-manage-archive-session" className="btn-sm btn-secondary" disabled={manageBusy} onClick={() => void applySessionAction('archive')}>Archive Session</button>
          <button id="server-manage-delete-session" className="btn-sm btn-danger" disabled={manageBusy} onClick={() => void applySessionAction('delete')}>Delete Session</button>
        </div>
        <pre id="server-manage-sessions" className="code-block text-dim" style={{ marginTop: '8px', minHeight: '72px' }}>{sessionsText}</pre>

        <div className="btn-row" style={{ marginTop: '10px' }}>
          <button id="server-manage-load-memory" className="btn-sm btn-secondary" disabled={manageBusy} onClick={() => void loadManageMemory()}>Load Memory</button>
        </div>
        <pre id="server-manage-memory" className="code-block text-dim" style={{ marginTop: '8px', minHeight: '72px' }}>{memoryText}</pre>
        <div id="server-manage-op-meta" className="manage-op-meta text-dim">{formatOperationMeta(opMeta)}</div>
        <div id="server-manage-msg">{renderMessage(manageMessage)}</div>
      </div>
    </section>
  );
}
