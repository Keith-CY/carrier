import { useMemo, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useNavigate, useParams } from 'react-router-dom';
import { apiGet, apiPost } from '../../lib/api';

type AgentStatus = Record<string, unknown>;

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

  const actionMutation = useMutation({
    mutationFn: async (action: 'start' | 'stop') => {
      await apiPost(`/api/v1/agents/${encodeURIComponent(agentId)}/${action}`, {});
      return action;
    },
    onSuccess: async (action) => {
      setLastActionMessage(`Agent ${action} requested.`);
      await statusQuery.refetch();
    },
    onError: (error) => {
      setLastActionMessage((error as Error).message);
    },
  });

  const content = useMemo(() => {
    if (!agentId) return { state: 'error', message: 'Error: missing agent id.' } as const;
    if (statusQuery.isLoading) return { state: 'loading', message: `Loading ${agentId}…` } as const;
    if (statusQuery.isError) {
      return { state: 'error', message: `Error: ${(statusQuery.error as Error).message}` } as const;
    }
    return { state: 'ready', payload: statusQuery.data || {} } as const;
  }, [agentId, statusQuery.data, statusQuery.error, statusQuery.isError, statusQuery.isLoading]);

  return (
    <section id="view-agent-detail" className="view">
      <div id="agent-detail-content">
        {content.state === 'loading' || content.state === 'error' ? (
          <>{content.message}</>
        ) : (
          <div className="card">
            <h3>{`Agent: ${agentId}`}</h3>
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
