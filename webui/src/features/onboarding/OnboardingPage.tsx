import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { apiGet, apiPost, type ChannelStatus, type ChannelStatusPayload, type ProviderAuthStatusPayload } from '../../lib/api';
import {
  agentDisplayName,
  ensureWizardStateForRoute,
  patchWizardState,
  resetWizardState,
  type WizardProvider,
} from './state';

type Step = 'welcome' | 'setup' | 'agents' | 'provider' | 'config' | 'install' | 'complete';

function flattenProviderCatalog(payload: any, authStatusPayload: ProviderAuthStatusPayload | null): WizardProvider[] {
  const categories = payload && payload.by_category && typeof payload.by_category === 'object' ? payload.by_category : {};
  const authStatuses = Array.isArray(authStatusPayload?.providers) ? authStatusPayload.providers : [];
  const authStatusByID = new Map(authStatuses.map((provider) => [String(provider.id || '').trim().toLowerCase(), provider] as const));
  const seen = new Set<string>();
  const providers: WizardProvider[] = [];
  Object.keys(categories).forEach((key) => {
    const items = Array.isArray(categories[key]) ? categories[key] : [];
    items.forEach((provider) => {
      const id = String(provider?.id || '').trim().toLowerCase();
      if (!id || seen.has(id)) return;
      seen.add(id);
      const authStatus = authStatusByID.get(id);
      providers.push({
        ...provider,
        configured: authStatus?.configured,
        reusable: authStatus?.reusable,
        hasSavedCredential: authStatus?.hasSavedCredential,
        credentialBackend: authStatus?.credentialBackend,
      });
    });
  });
  return providers.sort((left, right) => String(left?.name || left?.id || '').localeCompare(String(right?.name || right?.id || '')));
}

function collectEnvVars(rows: Array<{ key: string; value: string }>) {
  return rows.reduce<Record<string, string>>((acc, row) => {
    const key = String(row.key || '').trim();
    if (!key) return acc;
    acc[key] = String(row.value || '');
    return acc;
  }, {});
}

export function OnboardingPage({ step, addTargetAgent }: { step: Step; addTargetAgent?: string }) {
  const navigate = useNavigate();
  const params = useParams();
  const resolvedAddTargetAgent = addTargetAgent || String(params.agentId || '').trim().toLowerCase();
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

  const [welcomeStatus, setWelcomeStatus] = useState('Detecting daemon connection...');
  const [welcomeConnected, setWelcomeConnected] = useState(false);
  const [agents, setAgents] = useState<any[]>([]);
  const [agentsMsg, setAgentsMsg] = useState('Loading agents...');
  const [setupMsg, setSetupMsg] = useState('');
  const [pairMsg, setPairMsg] = useState('');
  const [channelOptions, setChannelOptions] = useState<ChannelStatus[]>([]);
  const [providers, setProviders] = useState<WizardProvider[]>([]);
  const [providerMsg, setProviderMsg] = useState('');
  const [providerLoading, setProviderLoading] = useState(true);
  const [carrierDefaultProvider, setCarrierDefaultProvider] = useState<any>(null);
  const [installMsg, setInstallMsg] = useState('');

  const selectedChannelStatus = useMemo(
    () => channelOptions.find((candidate) => String(candidate.id || '').trim().toLowerCase() === String(channel || '').trim().toLowerCase()) || null,
    [channel, channelOptions],
  );

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

  useEffect(() => {
    if (step !== 'welcome') return;
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
  }, [step]);

  useEffect(() => {
    if (step !== 'agents') return;
    void apiGet<any[]>('/api/v1/agents')
      .then((payload) => {
        const nextAgents = Array.isArray(payload) ? payload : [];
        setAgents(nextAgents);
        setAgentsMsg(nextAgents.length ? '' : 'No agents found.');
      })
      .catch((error) => {
        setAgents([]);
        setAgentsMsg(`Error loading agents: ${error.message}`);
      });
  }, [step]);

  useEffect(() => {
    if (step !== 'setup') return;
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
        setChannel((current) => {
          const currentID = String(current || '').trim().toLowerCase();
          if (currentID && nextChannels.some((candidate) => String(candidate.id || '').trim().toLowerCase() === currentID)) return current;
          return String(nextChannels[0]?.id || '');
        });
      })
      .catch((error) => {
        if (cancelled) return;
        setChannelOptions([]);
        setSetupMsg(`Error loading channels: ${error.message}`);
      });
    return () => {
      cancelled = true;
    };
  }, [step]);

  useEffect(() => {
    if (step !== 'setup' || !baseState.addMode || !selectedChannelStatus?.supportsPairing || !String(channel).trim()) return;
    void apiGet<any>(`/api/v1/pairing/sessions?provider=${encodeURIComponent(channel)}`)
      .then((payload) => {
        const sessions = Array.isArray(payload?.sessions) ? payload.sessions : [];
        const valid = sessions.filter((session) => String(session?.chatId || '').trim());
        if (valid.length === 1 && !channelChatId) {
          const chatId = String(valid[0].chatId).trim();
          setChannelChatId(chatId);
          setPairMsg(`Auto-selected Carrier paired user: ${chatId}`);
        }
      })
      .catch(() => {});
  }, [baseState.addMode, channel, channelChatId, selectedChannelStatus?.supportsPairing, step]);

  useEffect(() => {
    if (step !== 'provider') return;
    setProviderLoading(true);
    setProviderMsg('');
    void Promise.all([
      apiGet<any>('/api/v1/providers'),
      apiGet<ProviderAuthStatusPayload>('/api/v1/auth/providers'),
    ])
      .then(([payload, authStatusPayload]) => {
        const flattened = flattenProviderCatalog(payload, authStatusPayload);
        const defaultProvider = payload?.carrier_default_provider || null;
        setProviders(flattened);
        setCarrierDefaultProvider(defaultProvider);
        if (baseState.addMode && defaultProvider?.reusable) {
          const matched = flattened.find((provider) => String(provider.id || '').trim().toLowerCase() === String(defaultProvider.id || '').trim().toLowerCase());
          if (matched) setSelectedProvider(matched);
        }
      })
      .catch((error) => {
        setProviders([]);
        setCarrierDefaultProvider(null);
        setProviderMsg(`Error loading providers: ${error.message}`);
      })
      .finally(() => setProviderLoading(false));
  }, [baseState.addMode, step]);

  const addMode = baseState.addMode;
  const channelRequiresBotToken = selectedChannelStatus?.requiresBotToken ?? true;
  const channelRequiresWebhookSecret = !!selectedChannelStatus?.requiresWebhookSecret;
  const channelRequiresPairing = addMode && !!selectedChannelStatus?.supportsPairing;
  const selectedProviderReusable = !!selectedProvider?.reusable;
  const providerNextEnabled =
    !!selectedProvider &&
    (selectedProvider.auth_mode !== 'api_key' || !!providerApiKey.trim() || selectedProviderReusable || !!carrierDefaultProvider?.reusable);

  if (step === 'welcome') {
    return (
      <section id="view-welcome" className="view">
        <div className="card center-card">
          <h2>Welcome to Carrier</h2>
          <p className="text-dim">Detecting daemon connection…</p>
          <div id="welcome-status">{welcomeStatus}</div>
          <button
            id="welcome-continue"
            className={welcomeConnected ? '' : 'hidden'}
            type="button"
            onClick={() => navigate('/setup')}
          >
            Continue →
          </button>
        </div>
      </section>
    );
  }

  if (step === 'setup') {
    return (
      <section id="view-setup" className="view">
        <div className="steps-indicator" id="steps-indicator"></div>
        <div className="card">
          <h3 id="setup-title">{addMode ? `Step 1 — Choose Chat Channel for ${agentDisplayName(resolvedAddTargetAgent || '')}` : 'Step 1 — Configure Chat Channel'}</h3>
          <label htmlFor="provider" id="setup-provider-label">{addMode ? 'Channel' : 'Chat Channel'}</label>
          <select id="provider" value={channel} onChange={(event) => setChannel(event.target.value)}>
            <option value="">Select…</option>
            {channelOptions.map((option) => (
              <option key={option.id} value={option.id}>
                {option.displayName}
              </option>
            ))}
          </select>
          {selectedChannelStatus ? (
            <p id="setup-channel-summary" className="text-dim">
              {[
                selectedChannelStatus.supportsPairing ? 'Requires Carrier pairing' : 'No pairing required',
                selectedChannelStatus.requiresWebhookSecret ? 'Webhook secret required' : 'Webhook secret optional',
                selectedChannelStatus.configured ? 'Already configured in gateway' : '',
              ].filter(Boolean).join(' · ')}
            </p>
          ) : null}
          <label htmlFor="provider-token" id="setup-token-label">{addMode ? 'Channel Bot Token' : 'Channel Bot Token'}</label>
          <input id="provider-token" type="password" placeholder="Bot token" value={channelToken} onChange={(event) => setChannelToken(event.target.value)} />
          <label htmlFor="webhook-secret">
            Webhook Secret <span className="text-dim">({channelRequiresWebhookSecret ? 'required' : 'optional'})</span>
          </label>
          <input id="webhook-secret" type="text" placeholder="Webhook verification secret" value={webhookSecret} onChange={(event) => setWebhookSecret(event.target.value)} />
          {channelRequiresPairing ? (
            <div id="setup-telegram-pair">
              <p className="text-dim" id="setup-pair-instruction">
                {channelChatId ? `Paired chat id: ${channelChatId}` : 'Click Start Pairing to get a code, then send it in your bot chat.'}
              </p>
              <div className="btn-row">
                <button
                  id="setup-pair-use-carrier"
                  type="button"
                  className={`btn-sm${channelChatId ? '' : ' hidden'}`}
                  onClick={() => {
                    if (channelChatId) setPairMsg(`Using Carrier paired ${selectedChannelStatus?.displayName || 'channel'} user: ${channelChatId}`);
                  }}
                >
                  Use Carrier paired user (Recommended)
                </button>
                <button id="setup-pair-start" type="button" className="btn-secondary btn-sm">Start Pairing</button>
              </div>
              <div id="setup-pair-msg">{pairMsg}</div>
            </div>
          ) : null}
          <div className="btn-row">
            <button
              id="setup-btn"
              type="button"
              disabled={
                !channel.trim() ||
                (channelRequiresBotToken && !channelToken.trim()) ||
                (channelRequiresWebhookSecret && !webhookSecret.trim()) ||
                (channelRequiresPairing && !channelChatId)
              }
              onClick={() => {
                if (!channel.trim()) {
                  setSetupMsg(addMode ? 'Please choose a channel.' : 'Please choose a chat channel.');
                  return;
                }
                if (channelRequiresBotToken && !channelToken.trim()) {
                  setSetupMsg('Please enter channel bot token.');
                  return;
                }
                if (channelRequiresWebhookSecret && !webhookSecret.trim()) {
                  setSetupMsg(`Please enter ${selectedChannelStatus?.displayName || 'channel'} webhook secret.`);
                  return;
                }
                if (channelRequiresPairing && !channelChatId) {
                  setSetupMsg(`Please complete ${selectedChannelStatus?.displayName || 'channel'} pairing first to capture your chat id.`);
                  return;
                }
                setSetupMsg('');
                navigate(addMode ? '/provider' : '/agents');
              }}
            >
              Continue →
            </button>
          </div>
          <div id="setup-msg">{setupMsg}</div>
        </div>
      </section>
    );
  }

  if (step === 'agents') {
    return (
      <section id="view-agents" className="view">
        <div className="steps-indicator" id="steps-indicator-2"></div>
        <div className="card">
          <h3>Step 2 — Select Agent</h3>
          <ul className="agent-select-list" id="agent-pick">
            {agents.map((agent) => {
              const id = String(agent?.id || agent?.ID || agent?.name || '').trim();
              return (
                <li
                  key={id}
                  className={selectedAgent === id ? 'selected' : ''}
                  onClick={() => setSelectedAgent(id)}
                >
                  {id}
                </li>
              );
            })}
          </ul>
          <div className="btn-row">
            <button className="btn-secondary" id="agents-back" type="button" onClick={() => navigate('/setup')}>← Back</button>
            <button id="agents-next" type="button" disabled={!selectedAgent} onClick={() => navigate('/provider')}>Continue →</button>
          </div>
          <div id="agents-msg">{agentsMsg}</div>
        </div>
      </section>
    );
  }

  if (step === 'provider') {
    return (
      <section id="view-provider" className="view">
        <div className="steps-indicator" id="steps-indicator-p"></div>
        <div className="card">
          <h3>{addMode ? 'Step 2 — Select LLM Provider' : 'Step 3 — Select LLM Provider'}</h3>
          <p className="text-dim" id="provider-agent-name">{addMode ? `Adding: ${selectedAgent}` : `Configuring: ${selectedAgent}`}</p>
          <div id="provider-loading" className={`text-dim${providerLoading ? '' : ' hidden'}`}>Loading providers…</div>
          <div id="provider-add-choice" className={addMode ? '' : 'hidden'}>
            <h4>Carrier Provider</h4>
            <p id="provider-default-summary" className="text-dim">
              {carrierDefaultProvider?.reusable
                ? `Using Carrier default: ${carrierDefaultProvider.provider?.name || carrierDefaultProvider.id} (\`${carrierDefaultProvider.id}\`) · credential: ${carrierDefaultProvider.credential_backend || 'saved store'}.`
                : carrierDefaultProvider?.configured
                  ? `Carrier default: ${carrierDefaultProvider.id}, but cannot reuse now.`
                  : 'Carrier default provider is not configured.'}
            </p>
            <div className="btn-row">
              <button
                id="provider-use-default-continue"
                className={`btn-sm${carrierDefaultProvider?.reusable ? '' : ' hidden'}`}
                type="button"
                onClick={() => navigate(addMode ? '/install' : '/config')}
              >
                Use Carrier Provider (Recommended) →
              </button>
            </div>
          </div>
          <div id="provider-other-wrap" className={providers.length ? '' : 'hidden'}>
            <div id="provider-categories" className={providers.length ? '' : 'hidden'}>
              <div className="provider-category">
                <ul className="provider-list">
                  {providers.map((provider) => (
                    <li
                      key={provider.id}
                      className={`provider-item${selectedProvider?.id === provider.id ? ' selected' : ''}`}
                      onClick={() => setSelectedProvider(provider)}
                    >
                      <strong>{provider.name}</strong> <code>{provider.id}</code>
                      {provider.reusable ? <><br /><span className="text-dim">Reusable saved credential</span></> : null}
                      {!provider.reusable && provider.configured ? <><br /><span className="text-dim">Configured in environment</span></> : null}
                      {provider.example_model ? <br /> : null}
                      {provider.example_model ? <span className="text-dim">e.g. {provider.example_model}</span> : null}
                    </li>
                  ))}
                </ul>
              </div>
            </div>
          </div>
          <div id="provider-auth-section" className={`provider-auth-section${selectedProvider ? '' : ' hidden'}`} style={{ marginTop: '1rem' }}>
            <div id="provider-auth-label" className="text-dim">
              {selectedProvider?.auth_mode === 'api_key'
                ? selectedProviderReusable
                  ? `Carrier already has a reusable credential for ${selectedProvider?.name}. Paste a new API key only if you want to override it.`
                  : `Paste API key for ${selectedProvider?.name} (${selectedProvider?.env_var || ''}):`
                : selectedProvider
                  ? selectedProvider.reusable
                    ? `${selectedProvider.name} can reuse an existing Carrier credential.`
                    : `${selectedProvider.name} requires external authentication.`
                  : ''}
            </div>
            <input
              type="password"
              id="provider-api-key"
              placeholder="Paste API key here…"
              className={selectedProvider?.auth_mode === 'api_key' ? '' : 'hidden'}
              style={{ marginTop: '0.5rem' }}
              value={providerApiKey}
              onChange={(event) => setProviderApiKey(event.target.value)}
            />
            <div id="provider-auth-instructions" className={`text-dim${selectedProvider && selectedProvider.auth_mode !== 'api_key' ? '' : ' hidden'}`} style={{ marginTop: '0.5rem' }}>
              {selectedProvider && selectedProvider.auth_mode !== 'api_key'
                ? (addMode ? 'Paste access token below if you are not reusing Carrier credential.' : `Run: openclaw models auth login --provider ${selectedProvider.id}`)
                : ''}
            </div>
          </div>
          <div className="btn-row" style={{ marginTop: '1rem' }}>
            <button className="btn-secondary" id="provider-back" type="button" onClick={() => navigate(addMode ? '/setup' : '/agents')}>← Back</button>
            <button id="provider-skip" className="btn-secondary" type="button" onClick={() => navigate('/config')}>Skip →</button>
            <button id="provider-next" type="button" disabled={!providerNextEnabled} onClick={() => navigate(addMode ? '/install' : '/config')}>Continue →</button>
          </div>
          <div id="provider-msg">{providerMsg}</div>
        </div>
      </section>
    );
  }

  if (step === 'config') {
    return (
      <section id="view-config" className="view">
        <div className="steps-indicator" id="steps-indicator-3"></div>
        <div className="card">
          <h3>Step 4 — Environment Variables</h3>
          <p className="text-dim" id="config-agent-name">{selectedAgent ? `Configuring: ${selectedAgent}` : ''}</p>
          <div id="env-fields">
            {envRows.map((row, index) => (
              <div key={`env-${index}`} className="env-row">
                <input
                  type="text"
                  placeholder="KEY"
                  value={row.key}
                  onChange={(event) => {
                    const next = envRows.slice();
                    next[index] = { ...next[index], key: event.target.value };
                    setEnvRows(next);
                  }}
                />
                <input
                  type="text"
                  placeholder="VALUE"
                  value={row.value}
                  onChange={(event) => {
                    const next = envRows.slice();
                    next[index] = { ...next[index], value: event.target.value };
                    setEnvRows(next);
                  }}
                />
              </div>
            ))}
          </div>
          <div className="btn-row">
            <button id="add-env" className="btn-secondary btn-sm" type="button" onClick={() => setEnvRows((current) => current.concat({ key: '', value: '' }))}>+ Add Env Var</button>
          </div>
          <div className="btn-row" style={{ marginTop: '1rem' }}>
            <button className="btn-secondary" id="config-back" type="button" onClick={() => navigate('/provider')}>← Back</button>
            <button id="config-next" type="button" onClick={() => navigate('/install')}>Continue →</button>
          </div>
          <div id="config-msg"></div>
        </div>
      </section>
    );
  }

  if (step === 'install') {
    const summaryLines = [`Agent: ${selectedAgent}`];
    if (addMode && channel) summaryLines.push(`Channel: ${channel}`);
    if (selectedProvider?.name) summaryLines.push(`Provider: ${selectedProvider.name}`);

    return (
      <section id="view-install" className="view">
        <div className="steps-indicator" id="steps-indicator-4"></div>
        <div className="card center-card">
          <h3>Step 5 — Confirm Installation</h3>
          <p id="install-summary">{summaryLines.join('\n')}</p>
          <div className="btn-row">
            <button className="btn-secondary" id="install-back" type="button" onClick={() => navigate(addMode ? '/provider' : '/config')}>← Back</button>
            <button
              id="install-confirm"
              type="button"
              onClick={async () => {
                try {
                  setInstallMsg('Installing…');
                  const envVars = collectEnvVars(envRows);
                  if (webhookSecret) envVars.CARRIER_TELEGRAM_WEBHOOK_SECRET = webhookSecret;
                  const payload = await apiPost<any>('/api/v1/add', {
                    agentId: selectedAgent,
                    channel,
                    channelToken,
                    channelChatId,
                    providerId: selectedProvider?.id || '',
                    providerToken: providerApiKey,
                    reuseCredential: providerApiKey ? false : true,
                    envVars,
                  });
                  setLastAddResult(payload);
                  patchWizardState({ lastAddResult: payload });
                  setInstallMsg('');
                  navigate('/complete');
                } catch (error) {
                  setInstallMsg(`Error: ${(error as Error).message}`);
                }
              }}
            >
              Install &amp; Start →
            </button>
          </div>
          <div id="install-msg">{installMsg}</div>
        </div>
      </section>
    );
  }

  return (
    <section id="view-complete" className="view">
      <div className="card center-card">
        <h2 id="complete-title">✅ Setup Complete!</h2>
        <p className="text-dim">Your agent is being installed and started.</p>
        <p className="text-dim" id="complete-detail">
          {lastAddResult
            ? [
                lastAddResult.instanceId ? `Instance: ${lastAddResult.instanceId}` : '',
                lastAddResult.pairedChatId ? `Paired chat: ${lastAddResult.pairedChatId}` : '',
                lastAddResult.workspacePath ? `Workspace: ${lastAddResult.workspacePath}` : '',
                lastAddResult.configPath ? `Config: ${lastAddResult.configPath}` : '',
              ].filter(Boolean).join('\n')
            : ''}
        </p>
        <button
          id="complete-dashboard"
          type="button"
          onClick={() => {
            resetWizardState();
            navigate('/dashboard');
          }}
        >
          Go to Dashboard →
        </button>
      </div>
    </section>
  );
}
