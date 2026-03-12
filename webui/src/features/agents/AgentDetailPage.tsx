import { useMemo, useState } from 'react';
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
    enabled?: boolean;
  }>;
  mcp?: {
    servers?: Array<{
      name?: string;
      health?: string;
      enabled?: boolean;
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
    profiles?: Array<{
      profileName?: string;
      modelAlias?: string;
      modelId?: string;
      providerId?: string;
      providerKey?: string;
      protocolFamily?: string;
      baseUrl?: string;
      authMethod?: string;
      fallbackGroup?: string;
      aliasGroupSize?: number;
      primary?: boolean;
    }>;
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

export function AgentDetailPage() {
  const navigate = useNavigate();
  const params = useParams<{ agentId: string }>();
  const agentId = String(params.agentId || '').trim();
  const [lastActionMessage, setLastActionMessage] = useState('');

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

  const mcpToggleMutation = useMutation({
    mutationFn: async ({ serverName, enabled }: { serverName: string; enabled: boolean }) => {
      await apiPost(`/api/v1/agents/${encodeURIComponent(agentId)}/mcp/${encodeURIComponent(serverName)}`, { enabled });
      return { serverName, enabled };
    },
    onSuccess: async ({ serverName, enabled }) => {
      setLastActionMessage(`MCP server ${serverName} ${enabled ? 'enabled' : 'disabled'}.`);
      await capabilitiesQuery.refetch();
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
    return { state: 'ready', payload: statusPayload, capabilities: capabilitiesPayload, launcher: launcherPayload } as const;
  }, [
    agentId,
    capabilitiesQuery.data,
    capabilitiesQuery.error,
    capabilitiesQuery.isError,
    capabilitiesQuery.isLoading,
    launcherQuery.data,
    launcherQuery.isLoading,
    statusQuery.data,
    statusQuery.error,
    statusQuery.isError,
    statusQuery.isLoading,
  ]);

  return (
    <section id="view-agent-detail" className="view">
      <div id="agent-detail-content">
        {content.state === 'loading' || content.state === 'error' ? (
          <>{content.message}</>
        ) : (
          <div className="card">
            <h3>{`Agent: ${agentId}`}</h3>
            <div className="card-subtitle">Runtime Capabilities</div>
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
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
            {content.launcher?.modelSurface?.profiles && content.launcher.modelSurface.profiles.length ? (
              <div>
                <strong>Model Surface</strong>
                <div className="text-dim">
                  {content.launcher.modelSurface.defaultProfile ? `default=${content.launcher.modelSurface.defaultProfile}` : 'default=unconfigured'}
                </div>
                <ul className="compact-list">
                  {content.launcher.modelSurface.profiles.map((profile, index) => {
                    const label = String(profile.modelAlias || profile.profileName || `profile-${index + 1}`).trim();
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
                      </li>
                    );
                  })}
                </ul>
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
                      </span>
                      {skill.name ? (
                        <button
                          type="button"
                          className="btn-secondary"
                          disabled={skillToggleMutation.isPending}
                          onClick={() => skillToggleMutation.mutate({ skillName: String(skill.name), enabled: !skill.enabled })}
                        >
                          {skill.enabled ? 'Disable' : 'Enable'}
                        </button>
                      ) : null}
                    </li>
                  ))}
                </ul>
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
                        {` · visible=${server.visibleToolCount || 0} · hidden=${server.hiddenToolCount || 0}`}
                      </span>
                      {server.name && server.manageable ? (
                        <button
                          type="button"
                          className="btn-secondary"
                          disabled={mcpToggleMutation.isPending}
                          onClick={() => mcpToggleMutation.mutate({ serverName: String(server.name), enabled: !server.enabled })}
                        >
                          {server.enabled ? 'Disable MCP' : 'Enable MCP'}
                        </button>
                      ) : null}
                    </li>
                  ))}
                </ul>
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
