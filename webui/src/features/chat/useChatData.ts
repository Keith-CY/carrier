import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { useFeatures } from '../../app/useFeatures';
import { apiGet } from '../../lib/api';
import { normalizeExecutions } from '../dashboard/model';
import { useSettingsData } from '../settings/useSettingsData';
import { useWorkPageData } from '../work/useWorkData';

type ChatRole = 'assistant' | 'user' | 'system';

export type ChatMessage = {
  id: string;
  role: ChatRole;
  text: string;
  createdAt: string;
  sessionId?: string;
  requestId?: string;
};

type StarterPrompt = {
  label: string;
  prompt: string;
};

type PersistedChatState = {
  messages?: ChatMessage[];
  selectedProjectId?: string;
  providerOverride?: string;
  statusText?: string;
  sessionId?: string;
};

type UseChatDataOptions = {
  persistKey?: string;
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
    } catch {
      // ignore malformed frames
    }
  }
  return remaining;
}

function buildWelcomeMessage(): ChatMessage {
  return {
    id: 'assistant-welcome',
    role: 'assistant',
    createdAt: new Date().toISOString(),
    text: 'Tell me what you want done, ask what is currently active, or ask me to walk you through the next best step.',
  };
}

function normalizeRuntime(value: unknown) {
  const runtime = String(value || '').trim().toLowerCase();
  if (runtime === 'running' || runtime === 'healthy') return 'ready';
  if (runtime === 'error' || runtime === 'failed') return 'attention';
  return runtime || 'idle';
}

function buildStorageKey(persistKey: string) {
  return `carrier_chat_state:${persistKey}`;
}

function readPersistedState(persistKey?: string): PersistedChatState {
  if (!persistKey || typeof window === 'undefined') return {};
  try {
    const raw = window.localStorage.getItem(buildStorageKey(persistKey));
    if (!raw) return {};
    const parsed = JSON.parse(raw) as PersistedChatState;
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch {
    return {};
  }
}

function sanitizeMessages(value: unknown): ChatMessage[] {
  if (!Array.isArray(value)) return [];
  return value
    .map((item) => ({
      id: String(item?.id || '').trim(),
      role: item?.role === 'user' || item?.role === 'system' ? item.role : 'assistant',
      text: String(item?.text || ''),
      createdAt: String(item?.createdAt || new Date().toISOString()),
      sessionId: String(item?.sessionId || '').trim() || undefined,
      requestId: String(item?.requestId || '').trim() || undefined,
    }))
    .filter((item) => item.id && item.text);
}

export function useChatData(options: UseChatDataOptions = {}) {
  const persistedState = readPersistedState(options.persistKey);
  const { featureFlags, authz } = useFeatures();
  const [searchParams] = useSearchParams();
  const work = useWorkPageData();
  const settings = useSettingsData();
  const [input, setInput] = useState('');
  const [providerOverride, setProviderOverride] = useState(() => String(persistedState.providerOverride || '').trim());
  const [selectedProjectId, setSelectedProjectId] = useState(() => String(persistedState.selectedProjectId || '').trim());
  const [statusText, setStatusText] = useState(() => String(persistedState.statusText || '').trim() || 'Base agent ready.');
  const [messages, setMessages] = useState<ChatMessage[]>(() => {
    const persistedMessages = sanitizeMessages(persistedState.messages);
    return persistedMessages.length ? persistedMessages : [buildWelcomeMessage()];
  });
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  const lastInputRef = useRef('');
  const messageSeqRef = useRef(0);
  const sessionIdRef = useRef(String(persistedState.sessionId || '').trim());

  const instancesQuery = useQuery({
    queryKey: ['home', 'instances'],
    queryFn: () => apiGet<any>('/api/v1/instances'),
    enabled: featureFlags.remoteControlPlaneEnabled,
    retry: false,
  });

  const executionsQuery = useQuery({
    queryKey: ['home', 'executions'],
    queryFn: () => apiGet<any>('/api/v1/orchestrator/executions'),
    enabled: featureFlags.remoteControlPlaneEnabled && authz.permissions.viewExecutions,
    retry: false,
    refetchInterval: featureFlags.remoteControlPlaneEnabled && authz.permissions.viewExecutions ? 5000 : false,
  });

  const recentProjects = useMemo(() => {
    const projects = Array.isArray(work.query.data?.projects) ? work.query.data.projects : [];
    return projects.slice(0, 3);
  }, [work.query.data?.projects]);
  const projectOptions = useMemo(() => {
    const projects = Array.isArray(work.query.data?.projects) ? work.query.data.projects : [];
    return projects;
  }, [work.query.data?.projects]);

  const recentRuns = useMemo(() => {
    const runs = Array.isArray(work.query.data?.runs) ? work.query.data.runs : [];
    return runs.slice(0, 3);
  }, [work.query.data?.runs]);

  const runningAgents = useMemo(() => {
    const instances = Array.isArray(instancesQuery.data?.instances) ? instancesQuery.data.instances : Array.isArray(instancesQuery.data) ? instancesQuery.data : [];
    return instances.filter((item: any) => normalizeRuntime(item?.runtime_state || item?.runtimeState || item?.runtime) === 'ready').length;
  }, [instancesQuery.data]);

  const recentExecutions = useMemo(() => normalizeExecutions(executionsQuery.data).slice(0, 4), [executionsQuery.data]);
  const requestedProjectId = String(searchParams.get('project') || '').trim();

  const starterPrompts = useMemo<StarterPrompt[]>(() => {
    const leadProject = recentProjects[0];
    return [
      { label: 'Start a task', prompt: 'Help me start a new task with the minimum steps.' },
      { label: 'What can you do?', prompt: 'What can you do for me right now in Carrier?' },
      { label: 'Show my active work', prompt: 'What is currently active, blocked, or waiting for approval?' },
      {
        label: 'Continue last task',
        prompt: leadProject
          ? `Continue the most important next task for project ${leadProject.name}.`
          : 'Continue the last task I was working on.',
      },
    ];
  }, [recentProjects]);

  useEffect(() => {
    if (!requestedProjectId) return;
    if (!projectOptions.some((project) => project.id === requestedProjectId)) return;
    setSelectedProjectId((current) => current || requestedProjectId);
  }, [projectOptions, requestedProjectId]);

  useEffect(() => {
    if (!options.persistKey || typeof window === 'undefined') return;
    const payload: PersistedChatState = {
      messages,
      selectedProjectId,
      providerOverride,
      statusText,
      sessionId: sessionIdRef.current,
    };
    window.localStorage.setItem(buildStorageKey(options.persistKey), JSON.stringify(payload));
  }, [messages, options.persistKey, providerOverride, selectedProjectId, statusText]);

  const appendMessage = (role: ChatRole, text: string) => {
    messageSeqRef.current += 1;
    const id = `home-chat-${messageSeqRef.current}`;
    setMessages((current) => current.concat({
      id,
      role,
      text,
      createdAt: new Date().toISOString(),
    }));
    return id;
  };

  const replaceMessage = (messageId: string, update: Partial<ChatMessage>) => {
    setMessages((current) =>
      current.map((message) => (message.id === messageId ? { ...message, ...update } : message)),
    );
  };

  const appendDelta = (messageId: string, delta: string) => {
    setMessages((current) =>
      current.map((message) =>
        message.id === messageId ? { ...message, text: `${message.text}${delta}` } : message,
      ),
    );
  };

  const buildComposedMessage = (raw: string) => {
    const normalized = raw.trim();
    const project = projectOptions.find((item) => item.id === selectedProjectId);
    if (!project) return normalized;
    return `Project context: ${project.name}${project.sourceRef ? ` (${project.sourceRef})` : ''}\n\n${normalized}`;
  };

  const send = async (providedText?: string) => {
    const raw = String(providedText ?? input).trim();
    if (!raw) {
      setStatusText('Give the base agent a clear task or question first.');
      return;
    }

    const message = buildComposedMessage(raw);
    lastInputRef.current = raw;
    setInput('');
    appendMessage('user', raw);
    const assistantMessageId = appendMessage('assistant', '');

    if (abortRef.current) {
      abortRef.current.abort();
      abortRef.current = null;
    }

    const controller = new AbortController();
    abortRef.current = controller;
    setStatusText('Base agent is thinking…');

    try {
      const headers: Record<string, string> = { 'Content-Type': 'application/json' };
      const token = localStorage.getItem('carrier_token');
      if (token) headers.Authorization = `Bearer ${token}`;

      const response = await fetch('/api/v1/chat/stream', {
        method: 'POST',
        headers,
        signal: controller.signal,
        body: JSON.stringify({
          target: 'local',
          message,
          sessionId: sessionIdRef.current || '',
          provider: providerOverride.trim() || 'webui',
          chatId: sessionIdRef.current || '',
        }),
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
            appendDelta(assistantMessageId, String(payload?.delta || ''));
            return;
          }
          if (eventType === 'session') {
            sessionIdRef.current = String(payload?.sessionId || '').trim();
            replaceMessage(assistantMessageId, { sessionId: sessionIdRef.current });
            setStatusText(`Thread linked to session ${sessionIdRef.current}.`);
            return;
          }
          if (eventType === 'start' && payload?.requestId) {
            replaceMessage(assistantMessageId, { requestId: String(payload.requestId) });
            return;
          }
          if (eventType === 'finish') {
            setStatusText('Ready for the next step.');
          }
        });
      }
    } catch (error) {
      if ((error as Error).name === 'AbortError') {
        setStatusText('Stopped. You can send a new instruction now.');
      } else {
        replaceMessage(assistantMessageId, {
          text: `I could not complete that request: ${(error as Error).message}`,
        });
        setStatusText(`Chat failed: ${(error as Error).message}`);
      }
    } finally {
      if (abortRef.current === controller) abortRef.current = null;
    }
  };

  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key !== 'Enter' || event.shiftKey) return;
    event.preventDefault();
    void send();
  };

  return {
    input,
    setInput,
    messages,
    statusText,
    send,
    onKeyDown,
    providerOverride,
    setProviderOverride,
    selectedProjectId,
    setSelectedProjectId,
    recentProjects,
    projectOptions,
    recentRuns,
    recentExecutions,
    runningAgents,
    systemSummary: settings.summary,
    starterPrompts,
    featureFlags,
    advancedOpen,
    setAdvancedOpen,
    activeSessionId: sessionIdRef.current,
    isStreaming: !!abortRef.current,
    retryLast: () => {
      if (!lastInputRef.current) {
        setStatusText('There is nothing to retry yet.');
        return;
      }
      void send(lastInputRef.current);
    },
    clearConversation: () => {
      if (abortRef.current) abortRef.current.abort();
      abortRef.current = null;
      sessionIdRef.current = '';
      setMessages([buildWelcomeMessage()]);
      setStatusText('Started a fresh thread.');
    },
  };
}

export type ChatData = ReturnType<typeof useChatData>;
