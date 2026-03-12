import { useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import {
  agentDisplayName,
  type WizardProvider,
} from './state';
import { type Step } from './model';
import { useOnboardingActions } from './useOnboardingActions';
import { useOnboardingStepData } from './useOnboardingStepData';
import { useOnboardingWizardState } from './useOnboardingWizardState';

export function useOnboardingData({ step, addTargetAgent }: { step: Step; addTargetAgent?: string }) {
  const navigate = useNavigate();
  const params = useParams();
  const resolvedAddTargetAgent = addTargetAgent || String(params.agentId || '').trim().toLowerCase();
  const wizardState = useOnboardingWizardState(step, resolvedAddTargetAgent);
  const [setupMsg, setSetupMsg] = useState('');
  const [installMsg, setInstallMsg] = useState('');
  const stepData = useOnboardingStepData({
    step,
    addMode: wizardState.baseState.addMode,
    channel: wizardState.channel,
    channelChatId: wizardState.channelChatId,
    setChannelChatId: wizardState.setChannelChatId,
    setSelectedProvider: wizardState.setSelectedProvider,
  });
  const actions = useOnboardingActions({
    navigate,
    addMode: wizardState.baseState.addMode,
    channel: wizardState.channel,
    channelToken: wizardState.channelToken,
    channelChatId: wizardState.channelChatId,
    webhookSecret: wizardState.webhookSecret,
    selectedAgent: wizardState.selectedAgent,
    selectedProvider: wizardState.selectedProvider,
    providerApiKey: wizardState.providerApiKey,
    envRows: wizardState.envRows,
    setLastAddResult: wizardState.setLastAddResult,
    setSetupMsg,
    setInstallMsg,
  });
  const addMode = wizardState.baseState.addMode;
  const selectedProvider = wizardState.selectedProvider as WizardProvider | null;
  const providerNextEnabled = !!selectedProvider && (selectedProvider.auth_mode !== 'api_key' || !!wizardState.providerApiKey.trim() || !!stepData.carrierDefaultProvider?.reusable);

  return {
    step,
    navigate,
    resolvedAddTargetAgent,
    addMode,
    ...wizardState,
    ...stepData,
    setupMsg,
    installMsg,
    providerNextEnabled,
    agentDisplayName,
    ...actions,
  };
}

export type OnboardingData = ReturnType<typeof useOnboardingData>;
