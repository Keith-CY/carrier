import { KeyboardEvent, useRef, useState } from 'react';
import { apiPost } from '../../lib/api';
import { useFeatures } from '../../app/useFeatures';
import type { RemoteChatStatusType, RemoteChatStreamData, RemoteChatTargetData } from './types';

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

export function useRemoteChatStream(
  targets: Pick<RemoteChatTargetData, 'target' | 'hostId' | 'agentId' | 'profileId'>,
  updateStatus: (text: string, type?: RemoteChatStatusType) => void,
): RemoteChatStreamData {
  const { featureFlags } = useFeatures();
  const [input, setInput] = useState('');
  const [messages, setMessages] = useState<RemoteChatStreamData['messages']>([]);

  const abortRef = useRef<AbortController | null>(null);
  const lastInputRef = useRef('');
  const sessionIdRef = useRef('');
  const messageSeqRef = useRef(0);

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

  const send = async (providedText?: string) => {
    const message = String(providedText ?? input).trim();
    if (!message) {
      updateStatus('message is required.', 'error');
      return;
    }
    if (targets.target === 'remote' && !featureFlags.remoteControlPlaneEnabled) {
      updateStatus('Remote control plane is disabled by feature flag.', 'error');
      return;
    }
    if (targets.target === 'remote' && !featureFlags.remoteChatEnabled) {
      updateStatus('Remote chat is disabled by feature flag.', 'error');
      return;
    }
    if (targets.target === 'remote' && (!targets.hostId || !targets.agentId)) {
      updateStatus('host, instance and message are required for remote target.', 'error');
      return;
    }

    setInput('');
    lastInputRef.current = message;
    appendMessage('user', message);
    const assistantMessageId = appendMessage('assistant', '');

    if (targets.target === 'remote' && targets.profileId) {
      if (!featureFlags.providerBindingEnabled) {
        updateStatus('Provider binding is disabled by feature flag.', 'error');
        return;
      }
      try {
        await apiPost('/api/v1/provider-bindings', {
          id: `${targets.profileId}:instance:${targets.hostId}:${targets.agentId}`,
          profileId: targets.profileId,
          targetType: 'instance',
          targetId: `${targets.hostId}:${targets.agentId}`,
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
          target: targets.target,
          hostId: targets.target === 'remote' ? targets.hostId : '',
          agentId: targets.agentId,
          message,
          sessionId: sessionIdRef.current || '',
          provider: targets.target === 'local' ? 'webui' : '',
          chatId: targets.target === 'local' ? sessionIdRef.current || '' : '',
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

  return {
    input,
    setInput,
    messages,
    send,
    onEnter,
    resetSession: () => {
      sessionIdRef.current = '';
      updateStatus('Session reset. Next message starts a new session.', 'info');
    },
    cancelStream: () => {
      if (abortRef.current) abortRef.current.abort();
      else updateStatus('No active stream.', 'info');
    },
    retryLast: () => {
      if (!lastInputRef.current) {
        updateStatus('No previous message to retry.', 'info');
        return;
      }
      void send(lastInputRef.current);
    },
  };
}
