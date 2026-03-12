import { useCallback } from 'react';
import { useRemoteChatStatus } from './useRemoteChatStatus';
import { useRemoteChatTargetData } from './useRemoteChatTargetData';
import { useRemoteChatStream } from './useRemoteChatStream';
import type { RemoteChatData, RemoteChatStatusType } from './types';

export function useRemoteChatData() {
  const statusState = useRemoteChatStatus();

  const targets = useRemoteChatTargetData(statusState.updateStatus);
  const stream = useRemoteChatStream(targets, statusState.updateStatus);

  const onTargetChange = useCallback(
    (next: RemoteChatData['target']) => {
      stream.cancelStream();
      stream.resetSession();
      targets.onTargetChange(next);
    },
    [stream, targets],
  );

  return {
    ...targets,
    ...stream,
    ...statusState,
    onTargetChange,
  };
}

export type { RemoteChatData } from './types';
