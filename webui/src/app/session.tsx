import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';

type HealthState = {
  text: string;
  className: string;
};

type SessionToast = {
  id: number;
  text: string;
  status: string;
};

type SessionContextValue = {
  token: string;
  authenticated: boolean;
  loginError: string;
  health: HealthState;
  toasts: SessionToast[];
  login: (nextToken: string) => Promise<boolean>;
  logout: () => void;
  clearLoginError: () => void;
  dismissToast: (id: number) => void;
};

const DEFAULT_HEALTH: HealthState = {
  text: '⚪ auth required',
  className: 'badge badge-unknown',
};

const CHECKING_HEALTH: HealthState = {
  text: '⚪ checking…',
  className: 'badge badge-unknown',
};

const SessionContext = createContext<SessionContextValue | null>(null);

function readStoredToken(): string {
  if (typeof window === 'undefined') return '';
  return String(window.localStorage.getItem('carrier_token') || '').trim();
}

async function probeHealth(nextToken: string): Promise<Response> {
  const headers: Record<string, string> = {};
  if (nextToken) headers.Authorization = `Bearer ${nextToken}`;
  return fetch('/healthz', { headers });
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const [token, setToken] = useState(() => readStoredToken());
  const [authenticated, setAuthenticated] = useState(false);
  const [loginError, setLoginError] = useState('');
  const [health, setHealth] = useState<HealthState>(() => (readStoredToken() ? CHECKING_HEALTH : DEFAULT_HEALTH));
  const [toasts, setToasts] = useState<SessionToast[]>([]);
  const reconnectTimerRef = useRef<number | null>(null);
  const delegateEventSourceRef = useRef<EventSource | null>(null);
  const nextToastIdRef = useRef(1);

  const dismissToast = useCallback((id: number) => {
    setToasts((current) => current.filter((item) => item.id !== id));
  }, []);

  const pushToast = useCallback((text: string, status: string) => {
    const message = String(text || '').trim();
    if (!message) return;
    const id = nextToastIdRef.current++;
    setToasts((current) => current.concat({ id, text: message, status }));
    window.setTimeout(() => {
      setToasts((current) => current.filter((item) => item.id !== id));
    }, 7000);
  }, []);

  const disconnectDelegateEvents = useCallback(() => {
    if (reconnectTimerRef.current) {
      window.clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    if (!delegateEventSourceRef.current) return;
    try {
      delegateEventSourceRef.current.close();
    } catch {
      // no-op
    }
    delegateEventSourceRef.current = null;
  }, []);

  const logout = useCallback(() => {
    disconnectDelegateEvents();
    setAuthenticated(false);
    setToken('');
    setLoginError('');
    setHealth(DEFAULT_HEALTH);
    if (typeof window !== 'undefined') {
      window.localStorage.removeItem('carrier_token');
    }
    void queryClient.invalidateQueries();
  }, [disconnectDelegateEvents, queryClient]);

  const login = useCallback(async (nextToken: string) => {
    const candidate = String(nextToken || '').trim();
    if (!candidate) {
      setLoginError('Please enter a token.');
      return false;
    }
    try {
      const response = await probeHealth(candidate);
      if (!response.ok) {
        setAuthenticated(false);
        setLoginError('Invalid token or connection failed.');
        setHealth(DEFAULT_HEALTH);
        return false;
      }
      if (typeof window !== 'undefined') {
        window.localStorage.setItem('carrier_token', candidate);
      }
      setToken(candidate);
      setAuthenticated(true);
      setLoginError('');
      setHealth({ text: '🟢 online', className: 'badge badge-ok' });
      await queryClient.invalidateQueries();
      return true;
    } catch {
      setAuthenticated(false);
      setLoginError('Invalid token or connection failed.');
      setHealth({ text: '🔴 offline', className: 'badge badge-error' });
      return false;
    }
  }, [queryClient]);

  const clearLoginError = useCallback(() => {
    setLoginError('');
  }, []);

  useEffect(() => {
    let cancelled = false;

    const validateExistingToken = async () => {
      if (!token) {
        setAuthenticated(false);
        setHealth(DEFAULT_HEALTH);
        return;
      }
      try {
        setHealth(CHECKING_HEALTH);
        const response = await probeHealth(token);
        if (cancelled) return;
        if (!response.ok) {
          logout();
          setLoginError('Invalid token or connection failed.');
          return;
        }
        setAuthenticated(true);
        setLoginError('');
        setHealth({ text: '🟢 online', className: 'badge badge-ok' });
      } catch {
        if (cancelled) return;
        setAuthenticated(false);
        setHealth({ text: '🔴 offline', className: 'badge badge-error' });
      }
    };

    void validateExistingToken();
    return () => {
      cancelled = true;
    };
  }, [logout, token]);

  useEffect(() => {
    if (!authenticated || !token) return;
    const timer = window.setInterval(async () => {
      try {
        const response = await probeHealth(token);
        if (response.ok) {
          setHealth({ text: '🟢 online', className: 'badge badge-ok' });
          return;
        }
        if (response.status === 401 || response.status === 403) {
          setHealth(DEFAULT_HEALTH);
          return;
        }
        setHealth({ text: '🔴 offline', className: 'badge badge-error' });
      } catch {
        setHealth({ text: '🔴 offline', className: 'badge badge-error' });
      }
    }, 30000);
    return () => {
      window.clearInterval(timer);
    };
  }, [authenticated, token]);

  useEffect(() => {
    const handleAuthExpired = () => {
      logout();
    };
    window.addEventListener('carrier:auth-expired', handleAuthExpired);
    return () => {
      window.removeEventListener('carrier:auth-expired', handleAuthExpired);
    };
  }, [logout]);

  useEffect(() => {
    disconnectDelegateEvents();
    if (!authenticated || !token || typeof EventSource !== 'function') return;
    const connect = () => {
      const url = `/api/v1/webui/delegate/events?token=${encodeURIComponent(token)}`;
      try {
        const source = new EventSource(url);
        delegateEventSourceRef.current = source;
        source.onmessage = (event) => {
          if (!event?.data) return;
          try {
            const payload = JSON.parse(event.data);
            pushToast(String(payload?.message || payload?.summary || ''), String(payload?.status || ''));
          } catch {
            // no-op
          }
        };
        source.onerror = () => {
          disconnectDelegateEvents();
          if (!authenticated) return;
          reconnectTimerRef.current = window.setTimeout(connect, 2000);
        };
      } catch {
        reconnectTimerRef.current = window.setTimeout(connect, 2000);
      }
    };
    connect();
    return () => {
      disconnectDelegateEvents();
    };
  }, [authenticated, disconnectDelegateEvents, pushToast, token]);

  const value = useMemo<SessionContextValue>(() => ({
    token,
    authenticated,
    loginError,
    health,
    toasts,
    login,
    logout,
    clearLoginError,
    dismissToast,
  }), [authenticated, clearLoginError, dismissToast, health, login, loginError, logout, token, toasts]);

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession() {
  const value = useContext(SessionContext);
  if (!value) {
    throw new Error('useSession must be used within SessionProvider');
  }
  return value;
}
