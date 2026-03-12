import { useMemo } from 'react';
import type { HostRecord } from './model';

export function useSelectedHost(hosts: HostRecord[], selectedHostId: string) {
  return useMemo(
    () => hosts.find((host) => String(host?.id || '').trim() === selectedHostId) || null,
    [hosts, selectedHostId],
  );
}
