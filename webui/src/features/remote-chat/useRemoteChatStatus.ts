import { useState } from 'react';
import type { RemoteChatStatusType } from './types';

export function useRemoteChatStatus() {
  const [status, setStatus] = useState('');
  const [statusType, setStatusType] = useState<RemoteChatStatusType>('info');

  return {
    status,
    statusType,
    updateStatus(text: string, type: RemoteChatStatusType = 'info') {
      setStatus(text);
      setStatusType(type);
    },
  };
}
