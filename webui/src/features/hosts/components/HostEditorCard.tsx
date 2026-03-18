import type { HostsData } from '../useHostsData';
import { renderHostsMessage } from './shared';

export function HostEditorCard({ data }: { data: HostsData }) {
  const { canManageHosts, sshAliases, editor, setEditor, editingHostId, editorBusy, handleSaveHost, resetEditor, setServersMessage } = data;

  return (
    <div className="card">
      <h3>Add / Update Server</h3>
      <div className="form-grid hosts-editor-grid">
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
      <div id="servers-msg">{renderHostsMessage(data.serversMessage)}</div>
    </div>
  );
}
