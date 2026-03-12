import { useEffect, useState } from 'react';
import { apiGet } from '../../lib/api';
import type { Step } from './model';
import type { WizardProvider } from './state';
import { useOnboardingAgentsData } from './useOnboardingAgentsData';
import { useOnboardingProviderData } from './useOnboardingProviderData';
import { useOnboardingWelcomeData } from './useOnboardingWelcomeData';

export function useOnboardingStepData(args: {
  step: Step;
  addMode: boolean;
  channel: string;
  channelChatId: string;
  setChannelChatId: (value: string) => void;
  setSelectedProvider: (value: WizardProvider | null) => void;
}) {
  const [pairMsg, setPairMsg] = useState('');
  const welcomeData = useOnboardingWelcomeData(args.step === 'welcome');
  const agentsData = useOnboardingAgentsData(args.step === 'agents');
  const providerData = useOnboardingProviderData({
    enabled: args.step === 'provider',
    addMode: args.addMode,
    setSelectedProvider: args.setSelectedProvider,
  });

  useEffect(() => {
    if (args.step !== 'setup' || !args.addMode || String(args.channel).trim().toLowerCase() !== 'telegram') return;
    void apiGet<any>('/api/v1/pairing/sessions?provider=telegram')
      .then((payload) => {
        const sessions = Array.isArray(payload?.sessions) ? payload.sessions : [];
        const valid = sessions.filter((session) => session?.chatId && /^[0-9]+$/.test(String(session.chatId).trim()));
        if (valid.length === 1 && !args.channelChatId) {
          const chatId = String(valid[0].chatId).trim();
          args.setChannelChatId(chatId);
          setPairMsg(`Auto-selected Carrier paired Telegram user: ${chatId}`);
        }
      })
      .catch(() => {});
  }, [args.addMode, args.channel, args.channelChatId, args.setChannelChatId, args.step]);

  return {
    ...welcomeData,
    ...agentsData,
    pairMsg,
    setPairMsg,
    ...providerData,
  };
}
