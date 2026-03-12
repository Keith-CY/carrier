import { useEffect, useState } from 'react';
import { apiGet } from '../../lib/api';

export function useOnboardingAgentsData(enabled: boolean) {
  const [agents, setAgents] = useState<any[]>([]);
  const [agentsMsg, setAgentsMsg] = useState('Loading agents...');

  useEffect(() => {
    if (!enabled) return;
    void apiGet<any[]>('/api/v1/agents')
      .then((payload) => {
        const nextAgents = Array.isArray(payload) ? payload : [];
        setAgents(nextAgents);
        setAgentsMsg(nextAgents.length ? '' : 'No agents found.');
      })
      .catch((error) => {
        setAgents([]);
        setAgentsMsg(`Error loading agents: ${error.message}`);
      });
  }, [enabled]);

  return {
    agents,
    agentsMsg,
  };
}

