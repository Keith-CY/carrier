import { useEffect, useMemo, useState } from 'react';
import { apiGet, type ChannelStatus, type ChannelStatusPayload } from '../../lib/api';
import { shouldRequireChannelPairing, type Step } from './model';
import type { WizardProvider } from './state';
import { useOnboardingAgentsData } from './useOnboardingAgentsData';
import { useOnboardingProviderData } from './useOnboardingProviderData';
import { useOnboardingWelcomeData } from './useOnboardingWelcomeData';

export function useOnboardingStepData(args: {
  step: Step;
  addMode: boolean;
  addTargetAgent: string;
  channel: string;
  setChannel: (value: string | ((current: string) => string)) => void;
  channelChatId: string;
  setChannelChatId: (value: string) => void;
  setSelectedProvider: (value: WizardProvider | null) => void;
  setSetupMsg: (value: string) => void;
}) {
  const [pairMsg, setPairMsg] = useState('');
  const [channelOptions, setChannelOptions] = useState<ChannelStatus[]>([]);
  const welcomeData = useOnboardingWelcomeData(args.step === 'welcome');
  const agentsData = useOnboardingAgentsData(args.step === 'agents');
  const providerData = useOnboardingProviderData({
    enabled: args.step === 'provider',
    addMode: args.addMode,
    setSelectedProvider: args.setSelectedProvider,
  });
  const selectedChannelStatus = useMemo(
    () => channelOptions.find((candidate) => String(candidate.id || '').trim().toLowerCase() === String(args.channel || '').trim().toLowerCase()) || null,
    [args.channel, channelOptions],
  );
  const channelRequiresPairing = shouldRequireChannelPairing(args.addTargetAgent, !!selectedChannelStatus?.supportsPairing);

  useEffect(() => {
    if (args.step !== 'setup') return;
    let cancelled = false;
    void apiGet<ChannelStatusPayload>('/api/v1/channels')
      .then((payload) => {
        if (cancelled) return;
        const nextChannels = Array.isArray(payload?.channels)
          ? payload.channels
            .filter((candidate) => candidate?.supportsProviderSetup && candidate?.id !== 'webui')
            .sort((left, right) => String(left?.displayName || left?.id || '').localeCompare(String(right?.displayName || right?.id || '')))
          : [];
        setChannelOptions(nextChannels);
        args.setChannel((current) => {
          const currentID = String(current || '').trim().toLowerCase();
          if (currentID && nextChannels.some((candidate) => String(candidate.id || '').trim().toLowerCase() === currentID)) return current;
          return String(nextChannels[0]?.id || '');
        });
      })
      .catch((error) => {
        if (cancelled) return;
        setChannelOptions([]);
        args.setSetupMsg(`Error loading channels: ${error.message}`);
      });
    return () => {
      cancelled = true;
    };
  }, [args.setChannel, args.setSetupMsg, args.step]);

  useEffect(() => {
    if (args.step !== 'setup' || !args.addMode || !channelRequiresPairing || !String(args.channel).trim()) return;
    void apiGet<any>(`/api/v1/pairing/sessions?provider=${encodeURIComponent(args.channel)}`)
      .then((payload) => {
        const sessions = Array.isArray(payload?.sessions) ? payload.sessions : [];
        const valid = sessions.filter((session) => String(session?.chatId || '').trim());
        if (valid.length === 1 && !args.channelChatId) {
          const chatId = String(valid[0].chatId).trim();
          args.setChannelChatId(chatId);
          setPairMsg(`Auto-selected Carrier paired ${selectedChannelStatus?.displayName || 'channel'} user: ${chatId}`);
        }
      })
      .catch(() => {});
  }, [args.addMode, args.channel, args.channelChatId, args.setChannelChatId, args.step, channelRequiresPairing, selectedChannelStatus?.displayName]);

  return {
    ...welcomeData,
    ...agentsData,
    channelOptions,
    selectedChannelStatus,
    pairMsg,
    setPairMsg,
    ...providerData,
  };
}
