import { KeyboardEvent, useEffect, useRef, useState } from 'react';
import { useFeatures } from '../../app/useFeatures';
import { apiGet, apiPost } from '../../lib/api';

type RemoteChatMessage = {
  id: string;
  role: 'user' | 'assistant' | 'system';
  text: string;
};

type Option = {
  value: string;
  label: string;
};

function parseSSEFrames(buffer: string, onEvent: (payload: any) => void) {
  let remaining = buffer;
  for (;;) {
    const idx = remaining.indexOf('\n\n');
    if (idx < 0) break;
    const frame = remaining.slice(0, idx);
    remaining = remaining.slice(idx + 2);
    const lines = frame.split('\n');
    const dataLines = lines.filter((line) => line.startsWith('data:')).map((line) => line.slice(5).trim());
    if (!dataLines.length) continue;
    try {
      onEvent(JSON.parse(dataLines.join('\n')));
    } catch {}
  }
  return remaining;
}

export function RemoteChatPage() {
  const { featureFlags } = useFeatures();
  const [target, setTarget] = useState<'remote' | 'local'>('remote');
  const [hosts, setHosts] = useState<Option[]>([]);
  const [profiles, setProfiles] = useState<Option[]>([{ value: '', label: 'none' }]);
  const [instances, setInstances] = useState<Option[]>([]);
  const [hostId, setHostId] = useState('');
  const [agentId, setAgentId] = useState('');
  const [profileId, setProfileId] = useState('');
  const [input, setInput] = useState('');
  const [status, setStatus] = useState('');
  const [statusType, setStatusType] = useState<'info' | 'error' | 'success' | ''>('info');
  const [messages, setMessages] = useState<RemoteChatMessage[]>([]);

  const abortRef = useRef<AbortController | null>(null);
  const lastInputRef = useRef('');
  const sessionIdRef = useRef('');
  const messageSeqRef = useRef(0);

  const updateStatus = (text: string, type: 'info' | 'error' | 'success' | '' = 'info') => {
    setStatus(text);
    setStatusType(type);
  };

  const appendMessage = (role: 'user' | 'assistant' | 'system', text: string) => {
    messageSeqRef.current += 1;
    const id = `remote-chat-${messageSeqRef.current}`;
    setMessages((current) => current.concat({ id, role, text }));
    return id;
  };

  const appendMessageDelta = (messageId: string, delta: string) => {
    setMessages((current) =>
      current.map((message) =>
        message.id === messageId ? { ...message, text: `${message.text}${delta}` } : message,
      ),
    );
  };

  const loadRemoteTargets = async (preferredHostId?: string, preferredProfileId?: string) => {
    const [hostsPayload, profilesPayload] = await Promise.all([
      apiGet<any>('/api/v1/remote/hosts'),
      apiGet<any>('/api/v1/provider-profiles'),
    ]);
    const nextHosts = Array.isArray(hostsPayload?.hosts)
      ? hostsPayload.hosts.map((host: any) => ({
          value: String(host?.id || '').trim(),
          label: String(host?.name || host?.id || '').trim(),
        })).filter((host: Option) => host.value)
      : [];
    const nextProfiles = [{ value: '', label: 'none' }].concat(
      Array.isArray(profilesPayload?.profiles)
        ? profilesPayload.profiles.map((profile: any) => ({
            value: String(profile?.id || '').trim(),
            label: String(profile?.name || profile?.id || '').trim(),
          })).filter((profile: Option) => profile.value)
        : [],
    );
    setHosts(nextHosts);
    setProfiles(nextProfiles);
    const resolvedHostId = preferredHostId && nextHosts.some((item) => item.value === preferredHostId)
      ? preferredHostId
      : (nextHosts[0]?.value || '');
    const resolvedProfileId = preferredProfileId && nextProfiles.some((item) => item.value === preferredProfileId)
      ? preferredProfileId
      : '';
    setHostId(resolvedHostId);
    setProfileId(resolvedProfileId);
    return resolvedHostId;
  };

  const loadRemoteInstances = async (nextHostId: string, preferredAgentId?: string) => {
    if (!nextHostId) {
      setInstances([]);
      setAgentId('');
      return;
    }
    const payload = await apiGet<any>(`/api/v1/remote/hosts/${encodeURIComponent(nextHostId)}/instances`);
    const seen = new Set<string>();
    const nextInstances = Array.isArray(payload?.instances)
      ? payload.instances
          .map((instance: any) => {
            const value = String(instance?.agentId || instance?.agentID || instance?.id || 'main').trim();
            if (!value || seen.has(value)) return null;
            seen.add(value);
            const runtimeState = String(instance?.runtimeState || 'unknown').trim();
            return { value, label: `${value} (${runtimeState})` };
          })
          .filter((instance: Option | null): instance is Option => !!instance)
      : [];
    setInstances(nextInstances);
    const resolvedAgentId = preferredAgentId && nextInstances.some((item) => item.value === preferredAgentId)
      ? preferredAgentId
      : (nextInstances[0]?.value || '');
    setAgentId(resolvedAgentId);
  };

  const loadLocalInstances = async (preferredAgentId?: string) => {
    const payload = await apiGet<any>('/api/v1/instances');
    const nextInstances: Option[] = [{ value: '', label: 'base-agent (fallback)' }];
    if (Array.isArray(payload?.instances)) {
      payload.instances.forEach((instance: any) => {
        const runtimeAgentId = String(instance?.agent_id || instance?.agentID || instance?.type || '').trim();
        if (!runtimeAgentId) return;
        const instanceId = String(instance?.id || '').trim();
        const runtimeState = String(instance?.runtime_state || instance?.runtimeState || 'unknown').trim();
        nextInstances.push({
          value: runtimeAgentId,
          label: `${instanceId || runtimeAgentId} (${runtimeAgentId}, ${runtimeState})`,
        });
      });
    }
    setInstances(nextInstances);
    const resolvedAgentId = preferredAgentId && nextInstances.some((item) => item.value === preferredAgentId)
      ? preferredAgentId
      : '';
    setAgentId(resolvedAgentId);
  };

  const refreshTargets = async () => {
    if (target === 'local') {
      try {
        await loadLocalInstances(agentId);
        updateStatus('Local target selected. Choose a local instance or use base-agent fallback.', 'info');
      } catch (error) {
        updateStatus(`Load local instances failed: ${(error as Error).message}`, 'error');
      }
      return;
    }
    try {
      const resolvedHostId = await loadRemoteTargets(hostId, profileId);
      await loadRemoteInstances(resolvedHostId, agentId);
      updateStatus('Targets loaded.', 'info');
    } catch (error) {
      updateStatus(`Load targets failed: ${(error as Error).message}`, 'error');
    }
  };

  useEffect(() => {
    void refreshTargets();
    return () => {
      if (abortRef.current) {
        abortRef.current.abort();
        abortRef.current = null;
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target]);

  const send = async (providedText?: string) => {
    const message = String(providedText ?? input).trim();
    if (!message) {
      updateStatus('message is required.', 'error');
      return;
    }
    if (target === 'remote' && !featureFlags.remoteControlPlaneEnabled) {
      updateStatus('Remote control plane is disabled by feature flag.', 'error');
      return;
    }
    if (target === 'remote' && !featureFlags.remoteChatEnabled) {
      updateStatus('Remote chat is disabled by feature flag.', 'error');
      return;
    }
    if (target === 'remote' && (!hostId || !agentId)) {
      updateStatus('host, instance and message are required for remote target.', 'error');
      return;
    }

    setInput('');
    lastInputRef.current = message;
    appendMessage('user', message);
    const assistantMessageId = appendMessage('assistant', '');

    if (target === 'remote' && profileId) {
      if (!featureFlags.providerBindingEnabled) {
        updateStatus('Provider binding is disabled by feature flag.', 'error');
        return;
      }
      try {
        await apiPost('/api/v1/provider-bindings', {
          id: `${profileId}:instance:${hostId}:${agentId}`,
          profileId,
          targetType: 'instance',
          targetId: `${hostId}:${agentId}`,
          syncMode: 'always_push',
        });
      } catch (error) {
        updateStatus(`Profile apply failed: ${(error as Error).message}`, 'error');
        return;
      }
    }

    if (abortRef.current) {
      abortRef.current.abort();
      abortRef.current = null;
    }
    const controller = new AbortController();
    abortRef.current = controller;
    updateStatus('Streaming response...', 'info');

    try {
      const headers: Record<string, string> = { 'Content-Type': 'application/json' };
      const token = localStorage.getItem('carrier_token');
      if (token) headers.Authorization = `Bearer ${token}`;
      const response = await fetch('/api/v1/chat/stream', {
        method: 'POST',
        headers,
        body: JSON.stringify({
          target,
          hostId: target === 'remote' ? hostId : '',
          agentId,
          message,
          sessionId: sessionIdRef.current || '',
          provider: target === 'local' ? 'webui' : '',
          chatId: target === 'local' ? (sessionIdRef.current || '') : '',
        }),
        signal: controller.signal,
      });
      if (!response.ok) {
        throw new Error((await response.text()) || `chat failed (${response.status})`);
      }
      if (!response.body || !response.body.getReader) {
        throw new Error('streaming body is not supported in this browser');
      }
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      for (;;) {
        const step = await reader.read();
        if (step.done) break;
        buffer += decoder.decode(step.value, { stream: true });
        buffer = parseSSEFrames(buffer, (payload) => {
          const eventType = String(payload?.type || '').trim();
          if (eventType === 'text-delta') {
            appendMessageDelta(assistantMessageId, String(payload?.delta || ''));
            return;
          }
          if (eventType === 'session') {
            sessionIdRef.current = String(payload?.sessionId || '').trim();
            updateStatus(`Session: ${sessionIdRef.current}`, 'info');
            return;
          }
          if (eventType === 'finish') {
            updateStatus('Stream finished.', 'success');
          }
        });
      }
      parseSSEFrames(buffer, () => {});
      updateStatus('Stream finished.', 'success');
    } catch (error) {
      if ((error as Error).name === 'AbortError') {
        updateStatus('Stream cancelled.', 'info');
      } else {
        updateStatus(`Stream failed: ${(error as Error).message}`, 'error');
      }
    } finally {
      if (abortRef.current === controller) abortRef.current = null;
    }
  };

  const onEnter = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') void send();
  };

  return (
    <section id="view-remote-chat" className="view view-remote-chat-surface">
      <h2>Remote Chat</h2>
      <div className="card remote-chat-toolbar">
        <div className="form-grid">
          <div>
            <label htmlFor="remote-chat-target">Target</label>
            <select
              id="remote-chat-target"
              value={target}
              onChange={(event) => {
                const next = event.target.value === 'local' ? 'local' : 'remote';
                setTarget(next);
                sessionIdRef.current = '';
                updateStatus(next === 'remote' ? 'Remote target selected.' : 'Local target selected.', 'info');
              }}
            >
              <option value="remote">remote</option>
              <option value="local">local</option>
            </select>
          </div>
          <div>
            <label htmlFor="remote-chat-host">Host</label>
            <select
              id="remote-chat-host"
              value={hostId}
              disabled={target !== 'remote'}
              onChange={async (event) => {
                const nextHostId = event.target.value;
                setHostId(nextHostId);
                try {
                  await loadRemoteInstances(nextHostId, '');
                } catch (error) {
                  updateStatus(`Load instances failed: ${(error as Error).message}`, 'error');
                }
              }}
            >
              {hosts.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </div>
          <div>
            <label htmlFor="remote-chat-instance">Instance</label>
            <select id="remote-chat-instance" value={agentId} onChange={(event) => setAgentId(event.target.value)}>
              {instances.map((option) => <option key={option.value || option.label} value={option.value}>{option.label}</option>)}
            </select>
          </div>
          <div>
            <label htmlFor="remote-chat-profile">Profile</label>
            <select
              id="remote-chat-profile"
              value={profileId}
              disabled={target !== 'remote'}
              onChange={(event) => setProfileId(event.target.value)}
            >
              {profiles.map((option) => <option key={option.value || option.label} value={option.value}>{option.label}</option>)}
            </select>
          </div>
        </div>
        <div className="btn-row">
          <button id="remote-chat-refresh" className="btn-sm btn-secondary" type="button" onClick={() => void refreshTargets()}>
            Refresh Targets
          </button>
          <button
            id="remote-chat-reset-session"
            className="btn-sm btn-secondary"
            type="button"
            onClick={() => {
              sessionIdRef.current = '';
              updateStatus('Session reset. Next message starts a new session.', 'info');
            }}
          >
            New Session
          </button>
          <button
            id="remote-chat-cancel"
            className="btn-sm btn-secondary"
            type="button"
            onClick={() => {
              if (abortRef.current) abortRef.current.abort();
              else updateStatus('No active stream.', 'info');
            }}
          >
            Cancel Stream
          </button>
          <button
            id="remote-chat-retry"
            className="btn-sm btn-secondary"
            type="button"
            onClick={() => {
              if (!lastInputRef.current) {
                updateStatus('No previous message to retry.', 'info');
                return;
              }
              void send(lastInputRef.current);
            }}
          >
            Retry Last
          </button>
        </div>
        <p id="remote-chat-status" className={`text-dim${statusType ? ` msg-${statusType}` : ''}`}>{status}</p>
      </div>
      <div className="card remote-chat-thread">
        <div id="remote-chat-messages" className="chat-messages">
          {messages.map((message) => (
            <div key={message.id} className="chat-msg">
              <span className="sender">
                {message.role === 'user' ? 'You' : message.role === 'assistant' ? 'Agent' : 'Carrier'}:
              </span>
              <span className="body"> {message.text}</span>
            </div>
          ))}
        </div>
      </div>
      <div className="card remote-chat-compose">
        <div className="chat-input-row">
          <input
            id="remote-chat-input"
            type="text"
            placeholder="Message remote instance…"
            autoComplete="off"
            value={input}
            onChange={(event) => setInput(event.target.value)}
            onKeyDown={onEnter}
          />
          <button id="remote-chat-send" type="button" onClick={() => void send()}>Send</button>
        </div>
      </div>
    </section>
  );
}
