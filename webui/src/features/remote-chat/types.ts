import type { KeyboardEvent } from 'react';

export type RemoteChatMessage = {
  id: string;
  role: 'user' | 'assistant' | 'system';
  text: string;
};

export type Option = {
  value: string;
  label: string;
};

export type RemoteChatStatusType = 'info' | 'error' | 'success' | '';

export type RemoteChatTarget = 'remote' | 'local';

export type RemoteChatTargetData = {
  target: RemoteChatTarget;
  setTarget: (next: RemoteChatTarget) => void;
  hosts: Option[];
  profiles: Option[];
  instances: Option[];
  hostId: string;
  setHostId: (next: string) => void;
  agentId: string;
  setAgentId: (next: string) => void;
  profileId: string;
  setProfileId: (next: string) => void;
  refreshTargets: () => Promise<void>;
  onHostChange: (nextHostId: string) => Promise<void>;
  onTargetChange: (next: RemoteChatTarget) => void;
};

export type RemoteChatStreamData = {
  input: string;
  setInput: (next: string) => void;
  messages: RemoteChatMessage[];
  send: (providedText?: string) => Promise<void>;
  onEnter: (event: KeyboardEvent<HTMLInputElement>) => void;
  resetSession: () => void;
  cancelStream: () => void;
  retryLast: () => void;
};

export type RemoteChatData = RemoteChatTargetData &
  RemoteChatStreamData & {
    status: string;
    statusType: RemoteChatStatusType;
  };
