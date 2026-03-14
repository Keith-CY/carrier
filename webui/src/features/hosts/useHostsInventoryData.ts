import { useEffect, useState } from 'react';
import { apiGet } from '../../lib/api';
import { normalizeHosts, normalizeSSHAliases, type HostRecord, type MessageState } from './model';

type UseHostsInventoryDataArgs = {
  remoteControlPlaneEnabled: boolean;
  featuresLoading: boolean;
  canManageHosts: boolean;
};

export function useHostsInventoryData({
  remoteControlPlaneEnabled,
  featuresLoading,
  canManageHosts,
}: UseHostsInventoryDataArgs) {
  const [hosts, setHosts] = useState<HostRecord[]>([]);
  const [sshAliases, setSshAliases] = useState<string[]>([]);
  const [serversMessage, setServersMessage] = useState<MessageState>({ type: 'info', text: '' });

  async function refresh() {
    try {
      const [hostsPayload, aliasesPayload] = await Promise.all([
        apiGet<any>('/api/v1/remote/hosts'),
        apiGet<any>('/api/v1/remote/ssh-config-hosts').catch(() => ({ hosts: [] })),
      ]);
      setHosts(normalizeHosts(hostsPayload));
      setSshAliases(normalizeSSHAliases(aliasesPayload));
    } catch (error) {
      setServersMessage({ type: 'error', text: `Load failed: ${(error as Error).message}` });
      setHosts([]);
      setSshAliases([]);
    }
  }

  useEffect(() => {
    if (featuresLoading || !remoteControlPlaneEnabled) return;
    void refresh();
  }, [featuresLoading, remoteControlPlaneEnabled]);

  useEffect(() => {
    if (canManageHosts) return;
    setServersMessage({ type: 'info', text: 'Current role cannot modify remote hosts.' });
  }, [canManageHosts]);

  return {
    hosts,
    setHosts,
    sshAliases,
    serversMessage,
    setServersMessage,
    refresh,
  };
}

