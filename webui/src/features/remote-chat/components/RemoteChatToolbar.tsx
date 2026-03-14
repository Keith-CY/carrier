import type { RemoteChatData } from '../useRemoteChatData';

export function RemoteChatToolbar({ data }: { data: RemoteChatData }) {
  return (
    <div className="card remote-chat-toolbar">
      <div className="form-grid">
        <div>
          <label htmlFor="remote-chat-target">Target</label>
          <select
            id="remote-chat-target"
            value={data.target}
            onChange={(event) => data.onTargetChange(event.target.value === 'local' ? 'local' : 'remote')}
          >
            <option value="remote">remote</option>
            <option value="local">local</option>
          </select>
        </div>
        <div>
          <label htmlFor="remote-chat-host">Host</label>
          <select
            id="remote-chat-host"
            value={data.hostId}
            disabled={data.target !== 'remote'}
            onChange={async (event) => { await data.onHostChange(event.target.value); }}
          >
            {data.hosts.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
          </select>
        </div>
        <div>
          <label htmlFor="remote-chat-instance">Instance</label>
          <select id="remote-chat-instance" value={data.agentId} onChange={(event) => data.setAgentId(event.target.value)}>
            {data.instances.map((option) => <option key={option.value || option.label} value={option.value}>{option.label}</option>)}
          </select>
        </div>
        <div>
          <label htmlFor="remote-chat-profile">Profile</label>
          <select
            id="remote-chat-profile"
            value={data.profileId}
            disabled={data.target !== 'remote'}
            onChange={(event) => data.setProfileId(event.target.value)}
          >
            {data.profiles.map((option) => <option key={option.value || option.label} value={option.value}>{option.label}</option>)}
          </select>
        </div>
      </div>
      <div className="btn-row">
        <button id="remote-chat-refresh" className="btn-sm btn-secondary" type="button" onClick={() => void data.refreshTargets()}>
          Refresh Targets
        </button>
        <button id="remote-chat-reset-session" className="btn-sm btn-secondary" type="button" onClick={data.resetSession}>
          New Session
        </button>
        <button id="remote-chat-cancel" className="btn-sm btn-secondary" type="button" onClick={data.cancelStream}>
          Cancel Stream
        </button>
        <button id="remote-chat-retry" className="btn-sm btn-secondary" type="button" onClick={data.retryLast}>
          Retry Last
        </button>
      </div>
      <p id="remote-chat-status" className={`text-dim${data.statusType ? ` msg-${data.statusType}` : ''}`}>{data.status}</p>
    </div>
  );
}
