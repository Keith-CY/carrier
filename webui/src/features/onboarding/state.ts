export type WizardProvider = {
  id: string;
  name: string;
  auth_mode?: string;
  env_var?: string;
  example_model?: string;
  configured?: boolean;
  reusable?: boolean;
  hasSavedCredential?: boolean;
  credentialBackend?: string;
};

export type WizardState = {
  addMode: boolean;
  addTargetAgent: string;
  selectedAgent: string;
  channel: string;
  channelToken: string;
  channelChatId: string;
  webhookSecret: string;
  selectedProvider: WizardProvider | null;
  providerApiKey: string;
  envRows: Array<{ key: string; value: string }>;
  lastAddResult: any;
};

const DEFAULT_WIZARD_STATE: WizardState = {
  addMode: false,
  addTargetAgent: '',
  selectedAgent: '',
  channel: '',
  channelToken: '',
  channelChatId: '',
  webhookSecret: '',
  selectedProvider: null,
  providerApiKey: '',
  envRows: [{ key: '', value: '' }],
  lastAddResult: null,
};

let wizardState: WizardState = structuredClone(DEFAULT_WIZARD_STATE);

export function getWizardState() {
  return wizardState;
}

export function patchWizardState(patch: Partial<WizardState>) {
  wizardState = {
    ...wizardState,
    ...patch,
    envRows: patch.envRows ? patch.envRows.slice() : wizardState.envRows.slice(),
  };
  return wizardState;
}

export function resetWizardState() {
  wizardState = structuredClone(DEFAULT_WIZARD_STATE);
  return wizardState;
}

export function ensureWizardStateForRoute(route: string, addTargetAgent = '') {
  const normalizedRoute = String(route || '').trim().toLowerCase();
  if (addTargetAgent) {
    wizardState = {
      ...wizardState,
      addMode: true,
      addTargetAgent,
      selectedAgent: addTargetAgent,
      channel: wizardState.channel || 'telegram',
    };
    return wizardState;
  }
  if (normalizedRoute === 'config' && !wizardState.envRows.length) {
    wizardState = { ...wizardState, envRows: [{ key: '', value: '' }] };
  }
  return wizardState;
}

export function agentDisplayName(agentId: string) {
  const normalized = String(agentId || '').trim().toLowerCase();
  switch (normalized) {
    case 'picoclaw':
      return 'PicoClaw';
    case 'zeroclaw':
      return 'ZeroClaw';
    case 'openclaw':
      return 'OpenClaw';
    default:
      return normalized || 'Agent';
  }
}
