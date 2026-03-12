import { useEffect, useState } from 'react';
import { apiGet } from '../../lib/api';
import type { Option, RemoteChatStatusType, RemoteChatTarget, RemoteChatTargetData } from './types';

type UpdateStatus = (text: string, type?: RemoteChatStatusType) => void;

export function useRemoteChatTargetData(updateStatus: UpdateStatus): RemoteChatTargetData {
  const [target, setTarget] = useState<RemoteChatTarget>('remote');
  const [hosts, setHosts] = useState<Option[]>([]);
  const [profiles, setProfiles] = useState<Option[]>([{ value: '', label: 'none' }]);
  const [instances, setInstances] = useState<Option[]>([]);
  const [hostId, setHostId] = useState('');
  const [agentId, setAgentId] = useState('');
  const [profileId, setProfileId] = useState('');

  const loadRemoteTargets = async (preferredHostId?: string, preferredProfileId?: string) => {
    const [hostsPayload, profilesPayload] = await Promise.all([
      apiGet<any>('/api/v1/remote/hosts'),
      apiGet<any>('/api/v1/provider-profiles'),
    ]);
    const nextHosts = Array.isArray(hostsPayload?.hosts)
      ? hostsPayload.hosts
          .map((host: any) => ({
            value: String(host?.id || '').trim(),
            label: String(host?.name || host?.id || '').trim(),
          }))
          .filter((host: Option) => host.value)
      : [];
    const nextProfiles = [{ value: '', label: 'none' }].concat(
      Array.isArray(profilesPayload?.profiles)
        ? profilesPayload.profiles
            .map((profile: any) => ({
              value: String(profile?.id || '').trim(),
              label: String(profile?.name || profile?.id || '').trim(),
            }))
            .filter((profile: Option) => profile.value)
        : [],
    );
    setHosts(nextHosts);
    setProfiles(nextProfiles);
    const resolvedHostId =
      preferredHostId && nextHosts.some((item) => item.value === preferredHostId)
        ? preferredHostId
        : nextHosts[0]?.value || '';
    const resolvedProfileId =
      preferredProfileId && nextProfiles.some((item) => item.value === preferredProfileId)
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
    const resolvedAgentId =
      preferredAgentId && nextInstances.some((item) => item.value === preferredAgentId)
        ? preferredAgentId
        : nextInstances[0]?.value || '';
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
    const resolvedAgentId =
      preferredAgentId && nextInstances.some((item) => item.value === preferredAgentId)
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target]);

  return {
    target,
    setTarget,
    hosts,
    profiles,
    instances,
    hostId,
    setHostId,
    agentId,
    setAgentId,
    profileId,
    setProfileId,
    refreshTargets,
    onHostChange: async (nextHostId: string) => {
      setHostId(nextHostId);
      try {
        await loadRemoteInstances(nextHostId, '');
      } catch (error) {
        updateStatus(`Load instances failed: ${(error as Error).message}`, 'error');
      }
    },
    onTargetChange: (next: RemoteChatTarget) => {
      setTarget(next);
      updateStatus(next === 'remote' ? 'Remote target selected.' : 'Local target selected.', 'info');
    },
  };
}

