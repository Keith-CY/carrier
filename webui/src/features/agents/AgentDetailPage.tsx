import { useMemo, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useNavigate, useParams } from 'react-router-dom';
import { apiGet, apiPost } from '../../lib/api';

type AgentStatus = Record<string, unknown>;
type AgentCapabilities = {
  skills?: Array<{
    name?: string;
    summary?: string;
    enabled?: boolean;
  }>;
  mcp?: {
    servers?: Array<{
      name?: string;
      health?: string;
      visibleToolCount?: number;
      hiddenToolCount?: number;
    }>;
    visibleTools?: Array<{
      name?: string;
      description?: string;
    }>;
  };
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

  const actionMutation = useMutation({
    mutationFn: async (action: 'start' | 'stop') => {
      await apiPost(`/api/v1/agents/${encodeURIComponent(agentId)}/${action}`, {});
      return action;
    },
    onSuccess: async (action) => {
      setLastActionMessage(`Agent ${action} requested.`);
      await statusQuery.refetch();
      await capabilitiesQuery.refetch();
    },
    onError: (error) => {
      setLastActionMessage((error as Error).message);
    },
  });

  const content = useMemo(() => {
    if (!agentId) return { state: 'error', message: 'Error: missing agent id.' } as const;
    if (statusQuery.isLoading || capabilitiesQuery.isLoading) return { state: 'loading', message: `Loading ${agentId}…` } as const;
    if (statusQuery.isError) {
      return { state: 'error', message: `Error: ${(statusQuery.error as Error).message}` } as const;
    }
    if (capabilitiesQuery.isError) {
      return { state: 'error', message: `Error: ${(capabilitiesQuery.error as Error).message}` } as const;
    }
    return { state: 'ready', payload: statusQuery.data || {}, capabilities: capabilitiesQuery.data || {} } as const;
  }, [
    agentId,
    capabilitiesQuery.data,
    capabilitiesQuery.error,
    capabilitiesQuery.isError,
    capabilitiesQuery.isLoading,
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
            <div className="kv-grid">
              <div>
                <strong>Skills</strong>
                <ul className="compact-list">
                  {(content.capabilities.skills || []).map((skill) => (
                    <li key={String(skill.name || '')}>
                      <span>{skill.name || 'unknown-skill'}</span>
                      <span className="text-dim">{skill.enabled ? 'enabled' : 'disabled'}</span>
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
                        {server.health || 'unknown'} · visible={server.visibleToolCount || 0} · hidden={server.hiddenToolCount || 0}
                      </span>
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
