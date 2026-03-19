import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';

type BeforeInstallPromptEvent = Event & {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed'; platform: string }>;
};

type PWAContextValue = {
  canInstall: boolean;
  isStandalone: boolean;
  promptInstall: () => Promise<boolean>;
  dismissInstall: () => void;
  installDismissed: boolean;
};

const INSTALL_DISMISS_KEY = 'carrier_inbox_install_dismissed';
const PWAContext = createContext<PWAContextValue | null>(null);

function readDismissedState(): boolean {
  if (typeof window === 'undefined') return false;
  return window.localStorage.getItem(INSTALL_DISMISS_KEY) === '1';
}

function detectStandaloneMode(): boolean {
  if (typeof window === 'undefined') return false;
  if (window.matchMedia?.('(display-mode: standalone)')?.matches) return true;
  return Boolean((window.navigator as Navigator & { standalone?: boolean }).standalone);
}

export function PWAProvider({ children }: { children: ReactNode }) {
  const [promptEvent, setPromptEvent] = useState<BeforeInstallPromptEvent | null>(null);
  const [installDismissed, setInstallDismissed] = useState(() => readDismissedState());
  const [isStandalone, setIsStandalone] = useState(() => detectStandaloneMode());

  const dismissInstall = useCallback(() => {
    setInstallDismissed(true);
    if (typeof window !== 'undefined') {
      window.localStorage.setItem(INSTALL_DISMISS_KEY, '1');
    }
  }, []);

  const promptInstall = useCallback(async () => {
    if (!promptEvent) return false;
    try {
      await promptEvent.prompt();
      const choice = await promptEvent.userChoice;
      const accepted = String(choice?.outcome || '') === 'accepted';
      if (accepted && typeof window !== 'undefined') {
        window.localStorage.removeItem(INSTALL_DISMISS_KEY);
      }
      setPromptEvent(null);
      if (accepted) setInstallDismissed(false);
      return accepted;
    } catch {
      return false;
    }
  }, [promptEvent]);

  useEffect(() => {
    if (typeof window === 'undefined') return undefined;

    const handleBeforeInstallPrompt = (event: Event) => {
      event.preventDefault();
      setPromptEvent(event as BeforeInstallPromptEvent);
      setInstallDismissed(readDismissedState());
      setIsStandalone(detectStandaloneMode());
    };

    const handleInstalled = () => {
      setPromptEvent(null);
      setInstallDismissed(false);
      window.localStorage.removeItem(INSTALL_DISMISS_KEY);
      setIsStandalone(true);
    };

    const mediaQuery = window.matchMedia?.('(display-mode: standalone)');
    const syncStandalone = () => setIsStandalone(detectStandaloneMode());

    window.addEventListener('beforeinstallprompt', handleBeforeInstallPrompt);
    window.addEventListener('appinstalled', handleInstalled);
    mediaQuery?.addEventListener?.('change', syncStandalone);

    return () => {
      window.removeEventListener('beforeinstallprompt', handleBeforeInstallPrompt);
      window.removeEventListener('appinstalled', handleInstalled);
      mediaQuery?.removeEventListener?.('change', syncStandalone);
    };
  }, []);

  const value = useMemo<PWAContextValue>(() => ({
    canInstall: !!promptEvent && !installDismissed && !isStandalone,
    isStandalone,
    promptInstall,
    dismissInstall,
    installDismissed,
  }), [dismissInstall, installDismissed, isStandalone, promptInstall, promptEvent]);

  return <PWAContext.Provider value={value}>{children}</PWAContext.Provider>;
}

export function usePWAInstall() {
  const value = useContext(PWAContext);
  if (!value) {
    throw new Error('usePWAInstall must be used within PWAProvider');
  }
  return value;
}
