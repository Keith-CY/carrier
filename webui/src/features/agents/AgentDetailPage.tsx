import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useNavigate, useParams } from 'react-router-dom';
import { apiGet, apiPost } from '../../lib/api';

type AgentStatus = Record<string, unknown>;
type AgentCapabilities = {
  skillSummary?: {
    installedCount?: number;
    enabledCount?: number;
    disabledCount?: number;
  };
  skills?: Array<{
    name?: string;
    summary?: string;
    source?: string;
    version?: string;
    targetVersion?: string;
    health?: string;
    updateStatus?: string;
    updateAvailable?: boolean;
    enabled?: boolean;
  }>;
  mcp?: {
    servers?: Array<{
      name?: string;
      health?: string;
      enabled?: boolean;
      attached?: boolean;
      manageable?: boolean;
      visibleToolCount?: number;
      hiddenToolCount?: number;
    }>;
    visibleTools?: Array<{
      name?: string;
      description?: string;
    }>;
  };
};

type AgentSkillCatalogEntry = {
  name?: string;
  summary?: string;
  source?: string;
  version?: string;
  health?: string;
  updateStatus?: string;
  updateAvailable?: boolean;
  keywords?: string[];
  tags?: string[];
};

type AgentMCPServerDetail = {
  name?: string;
  health?: string;
  enabled?: boolean;
  attached?: boolean;
  manageable?: boolean;
  visibleToolCount?: number;
  hiddenToolCount?: number;
  healthDetail?: string;
  remediationHint?: string;
  configDigest?: string;
  configSummary?: string;
  visibleTools?: Array<{
    name?: string;
    description?: string;
  }>;
  hiddenTools?: Array<{
    name?: string;
    description?: string;
  }>;
};

type AgentModelSurfaceProfile = {
  profileName?: string;
  modelAlias?: string;
  modelId?: string;
  providerId?: string;
  providerKey?: string;
  protocolFamily?: string;
  baseUrl?: string;
  authMethod?: string;
  timeoutMs?: number;
  retryBudget?: number;
  fallbackStrategy?: string;
  fallbackGroup?: string;
  aliasGroupSize?: number;
  primary?: boolean;
};

type AgentModelProfileDraft = {
  profileName: string;
  modelAlias: string;
  modelId: string;
  providerId: string;
  baseUrl: string;
  authMethod: string;
  timeoutMs: string;
  retryBudget: string;
  fallbackStrategy: string;
};

type AgentLauncherSummary = {
  agentId?: string;
  status?: AgentStatus;
  heartbeat?: {
    state?: string;
    ageSeconds?: number;
    lastActivityAt?: string;
  };
  memory?: {
    contractId?: string;
    contractDigest?: string;
    syncState?: string;
  };
  providerReadiness?: {
    provider?: string;
    authMode?: string;
    credentialConfigured?: boolean;
    ready?: boolean;
    credentialBackend?: string;
  };
  modelSurface?: {
    defaultProfile?: string;
    profiles?: AgentModelSurfaceProfile[];
  };
  lastModelRun?: {
    requestedAlias?: string;
    requestedModel?: string;
    resolvedModel?: string;
    resolvedProfile?: string;
    fallbackGroup?: string;
    selectionStrategy?: string;
    selectionOrdinal?: number;
    overrideHit?: boolean;
    fallbackHit?: boolean;
    lastRunAt?: string;
  };
  cron?: {
    count?: number;
    nextRunAt?: string;
    lastRunAt?: string;
    lastResult?: string;
    jobs?: Array<{
      id?: string;
      prompt?: string;
      nextRunAt?: string;
      lastRunAt?: string;
      lastResult?: string;
      paused?: boolean;
      pausedAt?: string;
      history?: Array<{
        ranAt?: string;
        trigger?: string;
        result?: string;
        error?: string;
      }>;
    }>;
  };
  delegation?: {
    count?: number;
    jobs?: Array<{
      jobId?: string;
      task?: string;
      status?: string;
      summary?: string;
      result?: string;
      error?: string;
      updatedAt?: string;
    }>;
  };
  sessions?: {
    count?: number;
    sessions?: Array<{
      key?: string;
      messageCount?: number;
      summaryLength?: number;
      updatedAt?: string;
    }>;
  };
  session?: {
    instanceId?: string;
    channel?: string;
    isolation?: boolean;
    runtimeState?: string;
    pairedChatId?: string;
  };
  capabilities?: AgentCapabilities;
};

type AgentModelsSummary = {
  agentId?: string;
  instanceId?: string;
  configPath?: string;
  synced?: boolean;
  modelSurface?: {
    defaultProfile?: string;
    profiles?: AgentModelSurfaceProfile[];
  };
};

function buildProfileDraft(profile: AgentModelSurfaceProfile): AgentModelProfileDraft {
  return {
    profileName: String(profile.profileName || '').trim(),
    modelAlias: String(profile.modelAlias || '').trim(),
    modelId: String(profile.modelId || '').trim(),
    providerId: String(profile.providerId || '').trim(),
    baseUrl: String(profile.baseUrl || '').trim(),
    authMethod: String(profile.authMethod || '').trim(),
    timeoutMs: String(profile.timeoutMs || '').trim(),
    retryBudget: String(profile.retryBudget || '').trim(),
    fallbackStrategy: String(profile.fallbackStrategy || '').trim(),
  };
}

function buildLauncherRemediationMessages(launcher: AgentLauncherSummary | undefined): string[] {
  if (!launcher) return [];
  let messages: string[] = [];
  if (launcher.providerReadiness && launcher.providerReadiness.ready === false) {
    messages = appendUniqueMessage(messages, 'Provider authentication is not ready. Reconfigure credentials or switch to a ready profile.');
  }
  if (launcher.heartbeat && (launcher.heartbeat.state === 'stale' || launcher.heartbeat.state === 'expired')) {
    messages = appendUniqueMessage(messages, 'Launcher heartbeat is stale. Restart the agent or inspect the managed runtime.');
  }
  if (Array.isArray(launcher.cron?.jobs) && launcher.cron?.jobs.some((job) => !!job.paused)) {
    messages = appendUniqueMessage(messages, 'One or more cron jobs are paused. Resume or cancel them to restore scheduled automation.');
  }
  if (Array.isArray(launcher.capabilities?.skills) && launcher.capabilities.skills.some((skill) => !!skill.updateAvailable)) {
    messages = appendUniqueMessage(messages, 'One or more installed skills have updates pending. Review version drift and update pinned skills.');
  }
  if (Array.isArray(launcher.capabilities?.mcp?.servers) && launcher.capabilities.mcp.servers.some((server) => server.attached === false)) {
    messages = appendUniqueMessage(messages, 'One or more MCP servers are detached. Re-attach them before expecting tools to appear in runtime.');
  }
  if (launcher.session && String(launcher.session.runtimeState || '').trim() && String(launcher.session.runtimeState || '').trim() !== 'running') {
    messages = appendUniqueMessage(messages, 'Managed runtime is not running. Start the agent or inspect the launcher session.');
  }
  return messages;
}

function appendUniqueMessage(messages: string[], message: string): string[] {
  if (messages.includes(message)) {
    return messages;
  }
  return [...messages, message];
}

export function AgentDetailPage() {
  const navigate = useNavigate();
  const params = useParams<{ agentId: string }>();
  const agentId = String(params.agentId || '').trim();
  const [lastActionMessage, setLastActionMessage] = useState('');
  const [skillSearchQuery, setSkillSearchQuery] = useState('');
  const [skillSearchResults, setSkillSearchResults] = useState<AgentSkillCatalogEntry[]>([]);
  const [skillVersionDrafts, setSkillVersionDrafts] = useState<Record<string, string>>({});
  const [selectedMCPServerName, setSelectedMCPServerName] = useState('');
  const [mcpConfigDraft, setMcpConfigDraft] = useState('');
  const [editingProfileName, setEditingProfileName] = useState('');
  const [profileDraft, setProfileDraft] = useState<AgentModelProfileDraft | null>(null);

  const statusQuery = useQuery({
    queryKey: ['agent-detail', agentId],
    queryFn: () => apiGet<AgentStatus>(`/api/v1/agents/${encodeURIComponent(agentId)}/status`),
    enabled: !!agentId,
    retry: false,
  });
  const capabilitiesQuery = useQuery({
    queryKey: ['agent-capabilities', agentId],
    queryFn: () => apiGet<AgentCapabilities>(`/api/v1/agents/${encodeURIComponent(agentId)}/capabilities`),
    enabled: !!agentId,
    retry: false,
  });
  const launcherQuery = useQuery({
    queryKey: ['agent-launcher', agentId],
    queryFn: () => apiGet<AgentLauncherSummary>(`/api/v1/agents/${encodeURIComponent(agentId)}/launcher`),
    enabled: !!agentId,
    retry: false,
  });
  const modelsQuery = useQuery({
    queryKey: ['agent-models', agentId],
    queryFn: () => apiGet<AgentModelsSummary>(`/api/v1/agents/${encodeURIComponent(agentId)}/models`),
    enabled: !!agentId,
    retry: false,
  });

  const actionMutation = useMutation({
    mutationFn: async (action: 'start' | 'stop') => {
      await apiPost(`/api/v1/agents/${encodeURIComponent(agentId)}/${action}`, {});
      return action;
    },
    onSuccess: async (action) => {
      setLastActionMessage(`Agent ${action} requested.`);
      await statusQuery.refetch();
      await capabilitiesQuery.refetch();
      await launcherQuery.refetch();
    },
    onError: (error) => {
      setLastActionMessage((error as Error).message);
    },
  });

  const skillToggleMutation = useMutation({
    mutationFn: async ({ skillName, enabled }: { skillName: string; enabled: boolean }) => {
      await apiPost(`/api/v1/agents/${encodeURIComponent(agentId)}/skills/${encodeURIComponent(skillName)}`, { enabled });
      return { skillName, enabled };
    },
    onSuccess: async ({ skillName, enabled }) => {
      setLastActionMessage(`Skill ${skillName} ${enabled ? 'enabled' : 'disabled'}.`);
      await capabilitiesQuery.refetch();
      await launcherQuery.refetch();
    },
    onError: (error) => {
      setLastActionMessage((error as Error).message);
    },
  });

  const skillSearchMutation = useMutation({
    mutationFn: async (query: string) => {
      const trimmed = query.trim();
      const suffix = trimmed ? `?q=${encodeURIComponent(trimmed)}` : '';
      return apiGet<{ skills?: AgentSkillCatalogEntry[] }>(`/api/v1/agents/${encodeURIComponent(agentId)}/skills/search${suffix}`);
    },
    onSuccess: (payload) => {
      const skills = Array.isArray(payload.skills) ? payload.skills : [];
      setSkillSearchResults(skills);
      setLastActionMessage(skills.length ? `Found ${skills.length} skill result(s).` : 'No matching skills found.');
    },
    onError: (error) => {
      setSkillSearchResults([]);
      setLastActionMessage((error as Error).message);
    },
  });

  const skillInstallMutation = useMutation({
    mutationFn: async (skillName: string) => {
      return apiPost<AgentSkillCatalogEntry>(`/api/v1/agents/${encodeURIComponent(agentId)}/skills/install`, { name: skillName });
    },
    onSuccess: async (installed) => {
      const installedName = String(installed.name || '').trim() || 'unknown-skill';
      setLastActionMessage(`Installed skill ${installedName}.`);
      await capabilitiesQuery.refetch();
      await launcherQuery.refetch();
    },
    onError: (error) => {
      setLastActionMessage((error as Error).message);
    },
  });

  const skillUpdateMutation = useMutation({
    mutationFn: async ({ skillName, version }: { skillName: string; version: string }) => {
      return apiPost<AgentSkillCatalogEntry>(`/api/v1/agents/${encodeURIComponent(agentId)}/skills/update`, {
        name: skillName,
        version,
      });
    },
    onSuccess: async (updated) => {
      const updatedName = String(updated.name || '').trim() || 'unknown-skill';
      setLastActionMessage(`Updated skill ${updatedName}.`);
      setSkillVersionDrafts((current) => {
        const next = { ...current };
        delete next[updatedName];
        return next;
      });
      await capabilitiesQuery.refetch();
      await launcherQuery.refetch();
    },
    onError: (error) => {
      setLastActionMessage((error as Error).message);
    },
  });

  const skillUninstallMutation = useMutation({
    mutationFn: async (skillName: string) => {
      return apiPost<AgentSkillCatalogEntry>(`/api/v1/agents/${encodeURIComponent(agentId)}/skills/uninstall`, { name: skillName });
    },
    onSuccess: async (removed) => {
      const removedName = String(removed.name || '').trim() || 'unknown-skill';
      setLastActionMessage(`Removed skill ${removedName}.`);
      await capabilitiesQuery.refetch();
      await launcherQuery.refetch();
    },
    onError: (error) => {
      setLastActionMessage((error as Error).message);
    },
  });

  const mcpDetailQuery = useQuery({
    queryKey: ['agent-mcp-detail', agentId, selectedMCPServerName],
    queryFn: () => apiGet<AgentMCPServerDetail>(`/api/v1/agents/${encodeURIComponent(agentId)}/mcp/${encodeURIComponent(selectedMCPServerName)}`),
    enabled: !!agentId && !!selectedMCPServerName,
    retry: false,
  });

  useEffect(() => {
    if (mcpDetailQuery.data && selectedMCPServerName) {
      setMcpConfigDraft(String(mcpDetailQuery.data.configSummary || '').trim());
    }
  }, [mcpDetailQuery.data, selectedMCPServerName]);

  const mcpToggleMutation = useMutation({
    mutationFn: async ({ serverName, enabled }: { serverName: string; enabled: boolean }) => {
      await apiPost(`/api/v1/agents/${encodeURIComponent(agentId)}/mcp/${encodeURIComponent(serverName)}`, { enabled });
      return { serverName, enabled };
    },
    onSuccess: async ({ serverName, enabled }) => {
      setLastActionMessage(`MCP server ${serverName} ${enabled ? 'enabled' : 'disabled'}.`);
      await capabilitiesQuery.refetch();
      await launcherQuery.refetch();
      if (selectedMCPServerName === serverName) {
        await mcpDetailQuery.refetch();
      }
    },
    onError: (error) => {
      setLastActionMessage((error as Error).message);
    },
  });

  const mcpAttachMutation = useMutation({
    mutationFn: async ({ serverName, attached }: { serverName: string; attached: boolean }) => {
      const action = attached ? 'attach' : 'detach';
      return apiPost<AgentMCPServerDetail>(`/api/v1/agents/${encodeURIComponent(agentId)}/mcp/${encodeURIComponent(serverName)}/${action}`, {});
    },
    onSuccess: async (detail) => {
      const serverName = String(detail.name || selectedMCPServerName || 'unknown').trim();
      setLastActionMessage(`MCP server ${serverName} ${detail.attached === false ? 'detached' : 'attached'}.`);
      await capabilitiesQuery.refetch();
      await launcherQuery.refetch();
      if (selectedMCPServerName === serverName) {
        await mcpDetailQuery.refetch();
      }
    },
    onError: (error) => {
      setLastActionMessage((error as Error).message);
    },
  });

  const mcpConfigMutation = useMutation({
    mutationFn: async ({ serverName, config }: { serverName: string; config: string }) => {
      return apiPost<AgentMCPServerDetail>(`/api/v1/agents/${encodeURIComponent(agentId)}/mcp/${encodeURIComponent(serverName)}/config`, { config });
    },
    onSuccess: async (detail) => {
      const serverName = String(detail.name || selectedMCPServerName || 'unknown').trim();
      setLastActionMessage(`MCP config for ${serverName} updated.`);
      setMcpConfigDraft(String(detail.configSummary || '').trim());
      await mcpDetailQuery.refetch();
    },
    onError: (error) => {
      setLastActionMessage((error as Error).message);
    },
  });

  const cronMutation = useMutation({
    mutationFn: async ({ jobId, action }: { jobId: string; action: 'run' | 'pause' | 'resume' }) => {
      await apiPost(`/api/v1/agents/${encodeURIComponent(agentId)}/cron/${encodeURIComponent(jobId)}/${action}`, {});
      return { jobId, action };
    },
    onSuccess: async ({ jobId, action }) => {
      const actionLabel = action === 'run' ? 'run requested' : action === 'pause' ? 'paused' : 'resumed';
      setLastActionMessage(`Cron job ${jobId} ${actionLabel}.`);
      await launcherQuery.refetch();
    },
    onError: (error) => {
      setLastActionMessage((error as Error).message);
    },
  });

  const modelSyncMutation = useMutation({
    mutationFn: async () => apiPost<AgentModelsSummary>(`/api/v1/agents/${encodeURIComponent(agentId)}/models/sync`, {}),
    onSuccess: async () => {
      setLastActionMessage('Model surface synced.');
      await modelsQuery.refetch();
      await launcherQuery.refetch();
    },
    onError: (error) => {
      setLastActionMessage((error as Error).message);
    },
  });

  const modelDefaultMutation = useMutation({
    mutationFn: async (profileName: string) =>
      apiPost<AgentModelsSummary>(`/api/v1/agents/${encodeURIComponent(agentId)}/models/default`, { profileName }),
    onSuccess: async (updated) => {
      const updatedProfile = String(updated.modelSurface?.defaultProfile || '').trim();
      setLastActionMessage(updatedProfile ? `Default model profile set to ${updatedProfile}.` : 'Default model profile updated.');
      await modelsQuery.refetch();
      await launcherQuery.refetch();
    },
    onError: (error) => {
      setLastActionMessage((error as Error).message);
    },
  });

  const modelProfileMutation = useMutation({
    mutationFn: async (draft: AgentModelProfileDraft) =>
      apiPost<AgentModelsSummary>(`/api/v1/agents/${encodeURIComponent(agentId)}/models/profile`, {
        profileName: draft.profileName,
        modelAlias: draft.modelAlias,
        modelId: draft.modelId,
        providerId: draft.providerId,
        baseUrl: draft.baseUrl,
        authMethod: draft.authMethod,
        timeoutMs: draft.timeoutMs ? Number(draft.timeoutMs) : 0,
        retryBudget: draft.retryBudget ? Number(draft.retryBudget) : 0,
        fallbackStrategy: draft.fallbackStrategy,
      }),
    onSuccess: async (updated) => {
      const updatedProfile = String(profileDraft?.profileName || updated.modelSurface?.defaultProfile || '').trim();
      setLastActionMessage(updatedProfile ? `Model profile ${updatedProfile} updated.` : 'Model profile updated.');
      setEditingProfileName('');
      setProfileDraft(null);
      await modelsQuery.refetch();
      await launcherQuery.refetch();
    },
    onError: (error) => {
      setLastActionMessage((error as Error).message);
    },
  });

  const content = useMemo(() => {
    if (!agentId) return { state: 'error', message: 'Error: missing agent id.' } as const;
    const launcherPayload = launcherQuery.data || {};
    const statusPayload = statusQuery.data || launcherPayload.status || {};
    const capabilitiesPayload = capabilitiesQuery.data || launcherPayload.capabilities || {};
    const waitingForStatus = !statusQuery.data && !launcherPayload.status && (statusQuery.isLoading || launcherQuery.isLoading);
    const waitingForCapabilities = !capabilitiesQuery.data && !launcherPayload.capabilities && (capabilitiesQuery.isLoading || launcherQuery.isLoading);
    if (waitingForStatus || waitingForCapabilities) return { state: 'loading', message: `Loading ${agentId}…` } as const;
    if (statusQuery.isError && !launcherPayload.status) {
      return { state: 'error', message: `Error: ${(statusQuery.error as Error).message}` } as const;
    }
    if (capabilitiesQuery.isError && !launcherPayload.capabilities) {
      return { state: 'error', message: `Error: ${(capabilitiesQuery.error as Error).message}` } as const;
    }
    return {
      state: 'ready',
      payload: statusPayload,
      capabilities: capabilitiesPayload,
      launcher: launcherPayload,
      models: modelsQuery.data || {},
    } as const;
  }, [
    agentId,
    capabilitiesQuery.data,
    capabilitiesQuery.error,
    capabilitiesQuery.isError,
    capabilitiesQuery.isLoading,
    launcherQuery.data,
    launcherQuery.isLoading,
    modelsQuery.data,
    statusQuery.data,
    statusQuery.error,
    statusQuery.isError,
    statusQuery.isLoading,
  ]);
  const remediationMessages = useMemo(
    () => (content.state === 'ready' ? buildLauncherRemediationMessages(content.launcher) : []),
    [content],
  );
  const firstDetachedMCPServer = useMemo(() => {
    if (content.state !== 'ready') return '';
    const detached = (content.capabilities.mcp?.servers || []).find((server) => server.attached === false && server.name);
    return String(detached?.name || '').trim();
  }, [content]);
  const firstPausedCronJobID = useMemo(() => {
    if (content.state !== 'ready') return '';
    const paused = (content.launcher?.cron?.jobs || []).find((job) => job.paused && job.id);
    return String(paused?.id || '').trim();
  }, [content]);

  return (
    <section id="view-agent-detail" className="view">
      <div id="agent-detail-content">
        {content.state === 'loading' || content.state === 'error' ? (
          <>{content.message}</>
        ) : (
          <div className="card">
            <h3>{`Agent: ${agentId}`}</h3>
            <div className="card-subtitle">Runtime Capabilities</div>
            {remediationMessages.length ? (
              <div>
                <strong>Remediation</strong>
                <ul className="compact-list">
                  {remediationMessages.map((message) => (
                    <li key={message}>
                      <span className="text-dim">{message}</span>
                    </li>
                  ))}
                </ul>
                <div className="btn-row">
                  {content.launcher?.providerReadiness?.ready === false ? (
                    <button
                      type="button"
                      className="btn-secondary"
                      disabled={modelSyncMutation.isPending}
                      onClick={() => modelSyncMutation.mutate()}
                    >
                      Sync model surface
                    </button>
                  ) : null}
                  {content.launcher?.session?.runtimeState && content.launcher.session.runtimeState !== 'running' ? (
                    <button
                      type="button"
                      className="btn-secondary"
                      disabled={actionMutation.isPending}
                      onClick={() => actionMutation.mutate('start')}
                    >
                      Start runtime
                    </button>
                  ) : null}
                  {firstPausedCronJobID ? (
                    <button
                      type="button"
                      className="btn-secondary"
                      disabled={cronMutation.isPending}
                      onClick={() => cronMutation.mutate({ jobId: firstPausedCronJobID, action: 'resume' })}
                    >
                      Resume paused cron
                    </button>
                  ) : null}
                  {firstDetachedMCPServer ? (
                    <button
                      type="button"
                      className="btn-secondary"
                      disabled={mcpAttachMutation.isPending}
                      onClick={() => mcpAttachMutation.mutate({ serverName: firstDetachedMCPServer, attached: true })}
                    >
                      {`Attach ${firstDetachedMCPServer} MCP`}
                    </button>
                  ) : null}
                </div>
              </div>
            ) : null}
            {content.launcher && (content.launcher.heartbeat || content.launcher.providerReadiness || content.launcher.memory || content.launcher.session || content.launcher.cron) ? (
              <div className="kv-grid">
                <div>
                  <strong>Heartbeat</strong>
                  <div className="text-dim">
                    {content.launcher.heartbeat?.state || 'unknown'}
                    {typeof content.launcher.heartbeat?.ageSeconds === 'number' ? ` · age=${content.launcher.heartbeat.ageSeconds}s` : ''}
                    {content.launcher.heartbeat?.lastActivityAt ? ` · last=${content.launcher.heartbeat.lastActivityAt}` : ''}
                  </div>
                </div>
                <div>
                  <strong>Provider</strong>
                  <div className="text-dim">
                    {content.launcher.providerReadiness?.provider || 'unconfigured'}
                    {content.launcher.providerReadiness?.authMode ? ` · ${content.launcher.providerReadiness.authMode}` : ''}
                    {typeof content.launcher.providerReadiness?.ready === 'boolean' ? ` · ${content.launcher.providerReadiness.ready ? 'ready' : 'not ready'}` : ''}
                    {content.launcher.providerReadiness?.credentialConfigured ? ` · ${content.launcher.providerReadiness.credentialBackend || 'credential configured'}` : ''}
                  </div>
                </div>
                <div>
                  <strong>Memory</strong>
                  <div className="text-dim">
                    {content.launcher.memory?.contractId || 'none'}
                    {content.launcher.memory?.contractDigest ? ` · ${content.launcher.memory.contractDigest}` : ''}
                    {content.launcher.memory?.syncState ? ` · ${content.launcher.memory.syncState}` : ''}
                  </div>
                </div>
                <div>
                  <strong>Launcher Session</strong>
                  <div className="text-dim">
                    {content.launcher.session?.instanceId || 'n/a'}
                    {content.launcher.session?.channel ? ` · ${content.launcher.session.channel}` : ''}
                    {content.launcher.session?.runtimeState ? ` · ${content.launcher.session.runtimeState}` : ''}
                    {content.launcher.session?.isolation ? ' · isolated' : ''}
                    {content.launcher.session?.pairedChatId ? ` · paired=${content.launcher.session.pairedChatId}` : ''}
                  </div>
                </div>
                <div>
                  <strong>Cron</strong>
                  <div className="text-dim">
                    {typeof content.launcher.cron?.count === 'number' ? `${content.launcher.cron.count} job(s)` : 'none'}
                    {content.launcher.cron?.nextRunAt ? ` · next=${content.launcher.cron.nextRunAt}` : ''}
                    {content.launcher.cron?.lastRunAt ? ` · last=${content.launcher.cron.lastRunAt}` : ''}
                    {content.launcher.cron?.lastResult ? ` · ${content.launcher.cron.lastResult}` : ''}
                  </div>
                </div>
                <div>
                  <strong>Delegation</strong>
                  <div className="text-dim">
                    {typeof content.launcher.delegation?.count === 'number' ? `${content.launcher.delegation.count} job(s)` : 'none'}
                  </div>
                </div>
                <div>
                  <strong>Sessions</strong>
                  <div className="text-dim">
                    {typeof content.launcher.sessions?.count === 'number' ? `${content.launcher.sessions.count} session(s)` : 'none'}
                  </div>
                </div>
              </div>
            ) : null}
            {content.launcher?.cron?.jobs && content.launcher.cron.jobs.length ? (
              <div>
                <strong>Cron Jobs</strong>
                <ul className="compact-list">
                  {content.launcher.cron.jobs.map((job) => (
                    <li key={String(job.id || '')}>
                      <span>{job.id || 'unknown-job'}</span>
                      <span className="text-dim">
                        {job.prompt || 'no prompt'}
                        {job.nextRunAt ? ` · next=${job.nextRunAt}` : ''}
                        {job.lastRunAt ? ` · last=${job.lastRunAt}` : ''}
                        {job.lastResult ? ` · ${job.lastResult}` : ''}
                        {job.paused ? ' · paused' : ''}
                        {job.pausedAt ? ` · pausedAt=${job.pausedAt}` : ''}
                      </span>
                      {job.id ? (
                        <div className="btn-row">
                          <button
                            type="button"
                            className="btn-secondary"
                            disabled={cronMutation.isPending}
                            onClick={() => cronMutation.mutate({ jobId: String(job.id), action: 'run' })}
                          >
                            {`Run ${job.id} now`}
                          </button>
                          <button
                            type="button"
                            className="btn-secondary"
                            disabled={cronMutation.isPending || !!job.paused}
                            onClick={() => cronMutation.mutate({ jobId: String(job.id), action: 'pause' })}
                          >
                            {`Pause ${job.id}`}
                          </button>
                          <button
                            type="button"
                            className="btn-secondary"
                            disabled={cronMutation.isPending || !job.paused}
                            onClick={() => cronMutation.mutate({ jobId: String(job.id), action: 'resume' })}
                          >
                            {`Resume ${job.id}`}
                          </button>
                        </div>
                      ) : null}
                      {job.history && job.history.length ? (
                        <ul className="compact-list">
                          {job.history.map((run, index) => (
                            <li key={`${job.id || 'job'}-run-${index}`}>
                              <span>{run.ranAt || 'unknown-time'}</span>
                              <span className="text-dim">
                                {run.trigger || 'unknown-trigger'}
                                {run.result ? ` · ${run.result}` : ''}
                                {run.error ? ` · ${run.error}` : ''}
                              </span>
                            </li>
                          ))}
                        </ul>
                      ) : null}
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
            {content.launcher?.delegation?.jobs && content.launcher.delegation.jobs.length ? (
              <div>
                <strong>Recent Delegation Jobs</strong>
                <ul className="compact-list">
                  {content.launcher.delegation.jobs.map((job) => (
                    <li key={String(job.jobId || '')}>
                      <span>{job.jobId || 'unknown-job'}</span>
                      <span className="text-dim">
                        {job.task || 'no task'}
                        {job.status ? ` · ${job.status}` : ''}
                        {job.summary ? ` · ${job.summary}` : ''}
                        {job.result ? ` · ${job.result}` : ''}
                        {job.error ? ` · ${job.error}` : ''}
                        {job.updatedAt ? ` · ${job.updatedAt}` : ''}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
            {content.launcher?.sessions?.sessions && content.launcher.sessions.sessions.length ? (
              <div>
                <strong>Recent Sessions</strong>
                <ul className="compact-list">
                  {content.launcher.sessions.sessions.map((session) => (
                    <li key={String(session.key || '')}>
                      <span>{session.key || 'unknown-session'}</span>
                      <span className="text-dim">
                        {typeof session.messageCount === 'number' ? `${session.messageCount} messages` : 'message count unavailable'}
                        {typeof session.summaryLength === 'number' ? ` · summary=${session.summaryLength}` : ''}
                        {session.updatedAt ? ` · ${session.updatedAt}` : ''}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
            {((content.models?.modelSurface && content.models.modelSurface.profiles) || content.launcher?.modelSurface?.profiles)?.length ? (
              <div>
                <strong>Model Surface</strong>
                {(() => {
                  const currentDefaultProfile = String(content.models?.modelSurface?.defaultProfile || content.launcher?.modelSurface?.defaultProfile || '').trim();
                  const profiles = content.models?.modelSurface?.profiles || content.launcher?.modelSurface?.profiles || [];
                  return (
                    <>
                <div className="text-dim">
                  {currentDefaultProfile
                    ? `default=${currentDefaultProfile}`
                    : 'default=unconfigured'}
                  {content.models?.configPath ? ` · ${content.models.configPath}` : ''}
                </div>
                <div className="btn-row">
                  <button
                    type="button"
                    className="btn-secondary"
                    disabled={modelSyncMutation.isPending}
                    onClick={() => modelSyncMutation.mutate()}
                  >
                    Sync models
                  </button>
                </div>
                <ul className="compact-list">
                  {profiles.map((profile, index) => {
                    const label = String(profile.modelAlias || profile.profileName || `profile-${index + 1}`).trim();
                    const profileName = String(profile.profileName || '').trim();
                    const isDefault = profileName !== '' && profileName === currentDefaultProfile;
                    const isEditing = profileName !== '' && profileName === editingProfileName && profileDraft?.profileName === profileName;
                    return (
                      <li key={String(profile.profileName || label || index)}>
                        <span>{label}</span>
                        <span className="text-dim">
                          {profile.modelId || 'unknown-model'}
                          {profile.providerId ? ` · ${profile.providerId}` : ''}
                          {profile.protocolFamily ? ` · ${profile.protocolFamily}` : ''}
                          {profile.primary ? ' · primary' : ''}
                          {profile.fallbackGroup ? ` · ${profile.fallbackGroup}` : ''}
                          {profile.aliasGroupSize ? ` · group=${profile.aliasGroupSize}` : ''}
                        </span>
                        {profileName ? (
                          <div className="btn-row">
                            <button
                              type="button"
                              className="btn-secondary"
                              disabled={modelDefaultMutation.isPending || isDefault}
                              onClick={() => modelDefaultMutation.mutate(profileName)}
                            >
                              {isDefault ? `Default ${profileName}` : `Set default ${profileName}`}
                            </button>
                            <button
                              type="button"
                              className="btn-secondary"
                              disabled={modelProfileMutation.isPending}
                              onClick={() => {
                                setEditingProfileName(profileName);
                                setProfileDraft(buildProfileDraft(profile));
                              }}
                            >
                              {`Edit profile ${profileName}`}
                            </button>
                          </div>
                        ) : null}
                        {isEditing && profileDraft ? (
                          <div className="kv-grid">
                            <label>
                              <strong>{`Model alias for ${profileName}`}</strong>
                              <input
                                aria-label={`Model alias for ${profileName}`}
                                type="text"
                                value={profileDraft.modelAlias}
                                onChange={(event) => setProfileDraft({ ...profileDraft, modelAlias: event.target.value })}
                              />
                            </label>
                            <label>
                              <strong>{`Model ID for ${profileName}`}</strong>
                              <input
                                aria-label={`Model ID for ${profileName}`}
                                type="text"
                                value={profileDraft.modelId}
                                onChange={(event) => setProfileDraft({ ...profileDraft, modelId: event.target.value })}
                              />
                            </label>
                            <label>
                              <strong>{`Provider for ${profileName}`}</strong>
                              <input
                                aria-label={`Provider for ${profileName}`}
                                type="text"
                                value={profileDraft.providerId}
                                onChange={(event) => setProfileDraft({ ...profileDraft, providerId: event.target.value })}
                              />
                            </label>
                            <label>
                              <strong>{`Base URL for ${profileName}`}</strong>
                              <input
                                aria-label={`Base URL for ${profileName}`}
                                type="text"
                                value={profileDraft.baseUrl}
                                onChange={(event) => setProfileDraft({ ...profileDraft, baseUrl: event.target.value })}
                              />
                            </label>
                            <label>
                              <strong>{`Auth method for ${profileName}`}</strong>
                              <input
                                aria-label={`Auth method for ${profileName}`}
                                type="text"
                                value={profileDraft.authMethod}
                                onChange={(event) => setProfileDraft({ ...profileDraft, authMethod: event.target.value })}
                              />
                            </label>
                            <label>
                              <strong>{`Timeout ms for ${profileName}`}</strong>
                              <input
                                aria-label={`Timeout ms for ${profileName}`}
                                type="number"
                                value={profileDraft.timeoutMs}
                                onChange={(event) => setProfileDraft({ ...profileDraft, timeoutMs: event.target.value })}
                              />
                            </label>
                            <label>
                              <strong>{`Retry budget for ${profileName}`}</strong>
                              <input
                                aria-label={`Retry budget for ${profileName}`}
                                type="number"
                                value={profileDraft.retryBudget}
                                onChange={(event) => setProfileDraft({ ...profileDraft, retryBudget: event.target.value })}
                              />
                            </label>
                            <label>
                              <strong>{`Fallback strategy for ${profileName}`}</strong>
                              <input
                                aria-label={`Fallback strategy for ${profileName}`}
                                type="text"
                                value={profileDraft.fallbackStrategy}
                                onChange={(event) => setProfileDraft({ ...profileDraft, fallbackStrategy: event.target.value })}
                              />
                            </label>
                          </div>
                        ) : null}
                        {isEditing && profileDraft ? (
                          <div className="btn-row">
                            <button
                              type="button"
                              className="btn-primary"
                              disabled={modelProfileMutation.isPending}
                              onClick={() => modelProfileMutation.mutate(profileDraft)}
                            >
                              {`Save profile ${profileName}`}
                            </button>
                            <button
                              type="button"
                              className="btn-secondary"
                              disabled={modelProfileMutation.isPending}
                              onClick={() => {
                                setEditingProfileName('');
                                setProfileDraft(null);
                              }}
                            >
                              {`Cancel profile ${profileName}`}
                            </button>
                          </div>
                        ) : null}
                      </li>
                    );
                  })}
                </ul>
                    </>
                  );
                })()}
              </div>
            ) : null}
            {content.launcher?.lastModelRun ? (
              <div>
                <strong>Model Runtime Trace</strong>
                <div className="text-dim">
                  {content.launcher.lastModelRun.requestedAlias ? `requested=${content.launcher.lastModelRun.requestedAlias}` : 'requested=default'}
                  {content.launcher.lastModelRun.requestedModel ? ` · explicit=${content.launcher.lastModelRun.requestedModel}` : ''}
                  {content.launcher.lastModelRun.resolvedModel ? ` · resolved=${content.launcher.lastModelRun.resolvedModel}` : ''}
                  {content.launcher.lastModelRun.resolvedProfile ? ` · profile=${content.launcher.lastModelRun.resolvedProfile}` : ''}
                  {content.launcher.lastModelRun.fallbackGroup ? ` · group=${content.launcher.lastModelRun.fallbackGroup}` : ''}
                  {content.launcher.lastModelRun.selectionStrategy
                    ? ` · ${content.launcher.lastModelRun.selectionStrategy}${typeof content.launcher.lastModelRun.selectionOrdinal === 'number' ? `#${content.launcher.lastModelRun.selectionOrdinal}` : ''}`
                    : ''}
                  {content.launcher.lastModelRun.overrideHit ? ' · override hit' : ''}
                  {content.launcher.lastModelRun.fallbackHit ? ' · fallback hit' : ''}
                  {content.launcher.lastModelRun.lastRunAt ? ` · last=${content.launcher.lastModelRun.lastRunAt}` : ''}
                </div>
              </div>
            ) : null}
            <div className="kv-grid">
              <div>
                <strong>Skills</strong>
                {content.capabilities.skillSummary ? (
                  <div className="text-dim">
                    {content.capabilities.skillSummary.installedCount || 0} installed
                    {typeof content.capabilities.skillSummary.enabledCount === 'number' ? ` · ${content.capabilities.skillSummary.enabledCount} enabled` : ''}
                    {typeof content.capabilities.skillSummary.disabledCount === 'number' ? ` · ${content.capabilities.skillSummary.disabledCount} disabled` : ''}
                  </div>
                ) : null}
                <ul className="compact-list">
                  {(content.capabilities.skills || []).map((skill) => (
                    <li key={String(skill.name || '')}>
                      <span>{skill.name || 'unknown-skill'}</span>
                      <span className="text-dim">
                        {skill.enabled ? 'enabled' : 'disabled'}
                        {skill.summary ? ` · ${skill.summary}` : ''}
                        {skill.source ? ` · ${skill.source}` : ''}
                        {skill.version ? ` · ${skill.version}` : ''}
                        {skill.targetVersion ? ` · target=${skill.targetVersion}` : ''}
                        {skill.health ? ` · health=${skill.health}` : ''}
                        {skill.updateStatus ? ` · ${skill.updateStatus}` : ''}
                        {skill.updateAvailable ? ' · update available' : ''}
                      </span>
                      {skill.name ? (
                        <div className="btn-row">
                          <button
                            type="button"
                            className="btn-secondary"
                            disabled={skillToggleMutation.isPending}
                            onClick={() => skillToggleMutation.mutate({ skillName: String(skill.name), enabled: !skill.enabled })}
                          >
                            {skill.enabled ? 'Disable' : 'Enable'}
                          </button>
                          <button
                            type="button"
                            className="btn-secondary"
                            disabled={skillUninstallMutation.isPending}
                            onClick={() => skillUninstallMutation.mutate(String(skill.name))}
                          >
                            Uninstall
                          </button>
                          <input
                            type="text"
                            placeholder={`Pin version for ${String(skill.name)}`}
                            value={skillVersionDrafts[String(skill.name)] || ''}
                            onChange={(event) =>
                              setSkillVersionDrafts((current) => ({
                                ...current,
                                [String(skill.name)]: event.target.value,
                              }))
                            }
                          />
                          <button
                            type="button"
                            className="btn-secondary"
                            disabled={skillUpdateMutation.isPending}
                            onClick={() =>
                              skillUpdateMutation.mutate({
                                skillName: String(skill.name),
                                version: String(skillVersionDrafts[String(skill.name)] || '').trim(),
                              })
                            }
                          >
                            {`Update ${String(skill.name)}`}
                          </button>
                        </div>
                      ) : null}
                    </li>
                  ))}
                </ul>
                <div className="btn-row">
                  <input
                    type="text"
                    placeholder="Search skills"
                    value={skillSearchQuery}
                    onChange={(event) => setSkillSearchQuery(event.target.value)}
                  />
                  <button
                    type="button"
                    className="btn-secondary"
                    disabled={skillSearchMutation.isPending}
                    onClick={() => skillSearchMutation.mutate(skillSearchQuery)}
                  >
                    Search Skills
                  </button>
                </div>
                {skillSearchResults.length ? (
                  <ul className="compact-list">
                    {skillSearchResults.map((skill, index) => {
                      const skillName = String(skill.name || '').trim();
                      return (
                        <li key={skillName || `skill-search-${index}`}>
                          <span>{skillName || 'unknown-skill'}</span>
                          <span className="text-dim">
                            {skill.summary || 'no summary'}
                            {skill.tags?.length ? ` · tags=${skill.tags.join(', ')}` : ''}
                            {skill.source ? ` · ${skill.source}` : ''}
                            {skill.version ? ` · ${skill.version}` : ''}
                            {skill.health ? ` · health=${skill.health}` : ''}
                            {skill.updateStatus ? ` · ${skill.updateStatus}` : ''}
                            {skill.updateAvailable ? ' · update available' : ''}
                          </span>
                          {skillName ? (
                            <button
                              type="button"
                              className="btn-secondary"
                              disabled={skillInstallMutation.isPending}
                              onClick={() => skillInstallMutation.mutate(skillName)}
                            >
                              {`Install ${skillName}`}
                            </button>
                          ) : null}
                        </li>
                      );
                    })}
                  </ul>
                ) : null}
              </div>
              <div>
                <strong>MCP Servers</strong>
                <ul className="compact-list">
                  {((content.capabilities.mcp && content.capabilities.mcp.servers) || []).map((server) => (
                    <li key={String(server.name || '')}>
                      <span>{server.name || 'unknown-server'}</span>
                      <span className="text-dim">
                        {server.health || 'unknown'}
                        {typeof server.enabled === 'boolean' ? ` · ${server.enabled ? 'enabled' : 'disabled'}` : ''}
                        {typeof server.attached === 'boolean' ? ` · ${server.attached ? 'attached' : 'detached'}` : ''}
                        {` · visible=${server.visibleToolCount || 0} · hidden=${server.hiddenToolCount || 0}`}
                      </span>
                      {server.name && server.manageable ? (
                        <div className="btn-row">
                          <button
                            type="button"
                            className="btn-secondary"
                            disabled={mcpDetailQuery.isFetching && selectedMCPServerName === String(server.name)}
                            onClick={() => setSelectedMCPServerName(String(server.name))}
                          >
                            {`Inspect ${String(server.name)} MCP`}
                          </button>
                          <button
                            type="button"
                            className="btn-secondary"
                            disabled={mcpToggleMutation.isPending}
                            onClick={() => mcpToggleMutation.mutate({ serverName: String(server.name), enabled: !server.enabled })}
                          >
                            {server.enabled ? 'Disable MCP' : 'Enable MCP'}
                          </button>
                        </div>
                      ) : null}
                    </li>
                  ))}
                </ul>
                {selectedMCPServerName ? (
                  <div>
                    <strong>{`${selectedMCPServerName} MCP Detail`}</strong>
                    {mcpDetailQuery.isLoading ? (
                      <div className="text-dim">Loading MCP detail…</div>
                    ) : mcpDetailQuery.isError ? (
                      <div className="text-dim">{(mcpDetailQuery.error as Error).message}</div>
                    ) : mcpDetailQuery.data ? (
                      <>
                        <div className="text-dim">
                          {mcpDetailQuery.data.health || 'unknown'}
                          {typeof mcpDetailQuery.data.enabled === 'boolean' ? ` · ${mcpDetailQuery.data.enabled ? 'enabled' : 'disabled'}` : ''}
                          {typeof mcpDetailQuery.data.attached === 'boolean' ? ` · ${mcpDetailQuery.data.attached ? 'attached' : 'detached'}` : ''}
                          {` · visible=${mcpDetailQuery.data.visibleToolCount || 0} · hidden=${mcpDetailQuery.data.hiddenToolCount || 0}`}
                          {mcpDetailQuery.data.configDigest ? ` · ${mcpDetailQuery.data.configDigest}` : ''}
                        </div>
                        {mcpDetailQuery.data.configSummary ? <div className="text-dim">{mcpDetailQuery.data.configSummary}</div> : null}
                        {mcpDetailQuery.data.healthDetail ? <div className="text-dim">{mcpDetailQuery.data.healthDetail}</div> : null}
                        {mcpDetailQuery.data.remediationHint ? <div className="text-dim">{mcpDetailQuery.data.remediationHint}</div> : null}
                        <div className="btn-row">
                          <button
                            type="button"
                            className="btn-secondary"
                            disabled={mcpAttachMutation.isPending || mcpDetailQuery.data.attached === true}
                            onClick={() => mcpAttachMutation.mutate({ serverName: selectedMCPServerName, attached: true })}
                          >
                            {`Attach ${selectedMCPServerName}`}
                          </button>
                          <button
                            type="button"
                            className="btn-secondary"
                            disabled={mcpAttachMutation.isPending || mcpDetailQuery.data.attached === false}
                            onClick={() => mcpAttachMutation.mutate({ serverName: selectedMCPServerName, attached: false })}
                          >
                            {`Detach ${selectedMCPServerName}`}
                          </button>
                        </div>
                        <label>
                          <strong>{`${selectedMCPServerName} MCP config`}</strong>
                          <textarea
                            aria-label={`${selectedMCPServerName} MCP config`}
                            rows={4}
                            value={mcpConfigDraft}
                            onChange={(event) => setMcpConfigDraft(event.target.value)}
                          />
                        </label>
                        <div className="btn-row">
                          <button
                            type="button"
                            className="btn-secondary"
                            disabled={mcpConfigMutation.isPending}
                            onClick={() => mcpConfigMutation.mutate({ serverName: selectedMCPServerName, config: mcpConfigDraft })}
                          >
                            {`Save ${selectedMCPServerName} config`}
                          </button>
                        </div>
                        {mcpDetailQuery.data.visibleTools?.length ? (
                          <ul className="compact-list">
                            {mcpDetailQuery.data.visibleTools.map((tool) => (
                              <li key={`visible-${String(tool.name || '')}`}>
                                <span>{tool.name || 'unknown-tool'}</span>
                                {tool.description ? <span className="text-dim">{tool.description}</span> : null}
                              </li>
                            ))}
                          </ul>
                        ) : null}
                        {mcpDetailQuery.data.hiddenTools?.length ? (
                          <ul className="compact-list">
                            {mcpDetailQuery.data.hiddenTools.map((tool) => (
                              <li key={`hidden-${String(tool.name || '')}`}>
                                <span>{tool.name || 'unknown-tool'}</span>
                                {tool.description ? <span className="text-dim">{tool.description}</span> : null}
                              </li>
                            ))}
                          </ul>
                        ) : null}
                      </>
                    ) : null}
                  </div>
                ) : null}
              </div>
            </div>
            <div>
              <strong>Visible MCP Tools</strong>
              <ul className="compact-list">
                {((content.capabilities.mcp && content.capabilities.mcp.visibleTools) || []).map((tool) => (
                  <li key={String(tool.name || '')}>
                    <span>{tool.name || 'unknown-tool'}</span>
                    {tool.description ? <span className="text-dim">{tool.description}</span> : null}
                  </li>
                ))}
              </ul>
            </div>
            <pre className="log-box">{JSON.stringify(content.payload, null, 2)}</pre>
            {lastActionMessage ? <div className="text-dim">{lastActionMessage}</div> : null}
            <div className="btn-row">
              <button className="btn-secondary" type="button" onClick={() => navigate('/dashboard')}>
                ← Back
              </button>
              <button
                type="button"
                disabled={actionMutation.isPending}
                onClick={() => actionMutation.mutate('start')}
              >
                ▶ Start
              </button>
              <button
                className="btn-secondary"
                type="button"
                disabled={actionMutation.isPending}
                onClick={() => actionMutation.mutate('stop')}
              >
                ⏹ Stop
              </button>
            </div>
          </div>
        )}
      </div>
    </section>
  );
}
