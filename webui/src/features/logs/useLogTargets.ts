import { useEffect, useState } from 'react';
import { apiGet } from '../../lib/api';
import { buildLogOptions, type LogOption } from './model';

export function useLogTargets() {
  const [options, setOptions] = useState<LogOption[]>([]);
  const [selectedAgent, setSelectedAgent] = useState('');
  const [statusBase, setStatusBase] = useState('Select an agent and click Connect.');

  useEffect(() => {
    let cancelled = false;
    void Promise.allSettled([apiGet<any>('/api/v1/agents'), apiGet<any>('/api/v1/instances')]).then((results) => {
      if (cancelled) return;
      const agentsPayload = results[0].status === 'fulfilled' ? results[0].value : [];
      const instancesPayload = results[1].status === 'fulfilled' ? results[1].value : [];
      const normalized = buildLogOptions(agentsPayload, instancesPayload);
      setOptions(normalized.options);
      setStatusBase(normalized.emptyMessage);
      setSelectedAgent((current) => {
        if (current && normalized.options.some((option) => option.value === current)) return current;
        return normalized.options[0]?.value || '';
      });
    });
    return () => {
      cancelled = true;
    };
  }, []);

  return {
    options,
    selectedAgent,
    setSelectedAgent,
    statusBase,
    setStatusBase,
  };
}

