import { useEffect, useState } from 'react';

export function useOnboardingWelcomeData(enabled: boolean) {
  const [welcomeStatus, setWelcomeStatus] = useState('Detecting daemon connection...');
  const [welcomeConnected, setWelcomeConnected] = useState(false);

  useEffect(() => {
    if (!enabled) return;
    const token = localStorage.getItem('carrier_token');
    const headers: Record<string, string> = {};
    if (token) headers.Authorization = `Bearer ${token}`;
    fetch('/healthz', { headers })
      .then((response) => {
        if (!response.ok) throw new Error(`health check failed (${response.status})`);
        setWelcomeConnected(true);
        setWelcomeStatus('🟢 Daemon connected');
      })
      .catch(() => {
        setWelcomeConnected(false);
        setWelcomeStatus('🔴 Daemon unavailable');
      });
  }, [enabled]);

  return {
    welcomeStatus,
    welcomeConnected,
  };
}

