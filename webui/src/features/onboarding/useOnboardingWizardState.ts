import { useEffect, useMemo, useState } from 'react';
import {
  ensureWizardStateForRoute,
  patchWizardState,
  type WizardProvider,
} from './state';
import type { Step } from './model';

export function useOnboardingWizardState(step: Step, resolvedAddTargetAgent: string) {
  const baseState = useMemo(() => ensureWizardStateForRoute(step, resolvedAddTargetAgent), [resolvedAddTargetAgent, step]);
  const [selectedAgent, setSelectedAgent] = useState(baseState.selectedAgent);
  const [channel, setChannel] = useState(baseState.channel || (baseState.addMode ? 'telegram' : ''));
  const [channelToken, setChannelToken] = useState(baseState.channelToken);
  const [channelChatId, setChannelChatId] = useState(baseState.channelChatId);
  const [webhookSecret, setWebhookSecret] = useState(baseState.webhookSecret);
  const [selectedProvider, setSelectedProvider] = useState<WizardProvider | null>(baseState.selectedProvider);
  const [providerApiKey, setProviderApiKey] = useState(baseState.providerApiKey);
  const [envRows, setEnvRows] = useState(baseState.envRows);
  const [lastAddResult, setLastAddResult] = useState(baseState.lastAddResult);

  useEffect(() => {
    setSelectedAgent(baseState.selectedAgent);
    setChannel(baseState.channel || (baseState.addMode ? 'telegram' : ''));
    setChannelToken(baseState.channelToken);
    setChannelChatId(baseState.channelChatId);
    setWebhookSecret(baseState.webhookSecret);
    setSelectedProvider(baseState.selectedProvider);
    setProviderApiKey(baseState.providerApiKey);
    setEnvRows(baseState.envRows);
    setLastAddResult(baseState.lastAddResult);
  }, [baseState]);

  useEffect(() => {
    patchWizardState({
      addMode: baseState.addMode,
      addTargetAgent: baseState.addTargetAgent || '',
      selectedAgent,
      channel,
      channelToken,
      channelChatId,
      webhookSecret,
      selectedProvider,
      providerApiKey,
      envRows,
      lastAddResult,
    });
  }, [baseState.addMode, baseState.addTargetAgent, channel, channelChatId, channelToken, envRows, lastAddResult, providerApiKey, selectedAgent, selectedProvider, webhookSecret]);

  return {
    baseState,
    selectedAgent,
    setSelectedAgent,
    channel,
    setChannel,
    channelToken,
    setChannelToken,
    channelChatId,
    setChannelChatId,
    webhookSecret,
    setWebhookSecret,
    selectedProvider,
    setSelectedProvider,
    providerApiKey,
    setProviderApiKey,
    envRows,
    setEnvRows,
    lastAddResult,
    setLastAddResult,
  };
}
