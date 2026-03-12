import { apiPost } from '../../lib/api';
import { resetWizardState, patchWizardState } from './state';
import { collectEnvVars } from './model';
import type { WizardProvider } from './state';

export function useOnboardingActions(args: {
  navigate: (path: string) => void;
  addMode: boolean;
  channel: string;
  channelToken: string;
  channelChatId: string;
  webhookSecret: string;
  selectedAgent: string;
  selectedProvider: WizardProvider | null;
  providerApiKey: string;
  envRows: Array<{ key: string; value: string }>;
  setLastAddResult: (value: any) => void;
  setSetupMsg: (value: string) => void;
  setInstallMsg: (value: string) => void;
}) {
  return {
    nextFromSetup: () => {
      if (!args.channel.trim()) {
        args.setSetupMsg(args.addMode ? 'Please choose a channel.' : 'Please choose a chat channel.');
        return;
      }
      if (!args.channelToken.trim()) {
        args.setSetupMsg('Please enter channel bot token.');
        return;
      }
      if (args.addMode && args.channel === 'telegram' && !args.channelChatId) {
        args.setSetupMsg('Please complete Telegram pairing first to capture your chat id.');
        return;
      }
      args.setSetupMsg('');
      args.navigate(args.addMode ? '/provider' : '/agents');
    },
    installAndStart: async () => {
      try {
        args.setInstallMsg('Installing…');
        const envVars = collectEnvVars(args.envRows);
        if (args.webhookSecret) envVars.CARRIER_TELEGRAM_WEBHOOK_SECRET = args.webhookSecret;
        const payload = await apiPost<any>('/api/v1/add', {
          agentId: args.selectedAgent,
          channel: args.channel,
          channelToken: args.channelToken,
          channelChatId: args.channelChatId,
          providerId: args.selectedProvider?.id || '',
          providerToken: args.providerApiKey,
          reuseCredential: args.providerApiKey ? false : true,
          envVars,
        });
        args.setLastAddResult(payload);
        patchWizardState({ lastAddResult: payload });
        args.setInstallMsg('');
        args.navigate('/complete');
      } catch (error) {
        args.setInstallMsg(`Error: ${(error as Error).message}`);
      }
    },
    finish: () => {
      resetWizardState();
      args.navigate('/dashboard');
    },
  };
}
