import type { ReactNode } from 'react';

export function renderHostsMessage(message: { type: 'info' | 'success' | 'error'; text: string }): ReactNode {
  if (!message.text) return null;
  return <p className={`msg-${message.type}`}>{message.text}</p>;
}
