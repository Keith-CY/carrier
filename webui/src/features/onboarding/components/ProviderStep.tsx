import type { OnboardingData } from '../useOnboardingData';

export function ProviderStep({ data }: { data: OnboardingData }) {
  return (
    <section id="view-provider" className="view">
      <div className="steps-indicator" id="steps-indicator-p"></div>
      <div className="card">
        <h3>{data.addMode ? 'Step 2 — Select LLM Provider' : 'Step 3 — Select LLM Provider'}</h3>
        <p className="text-dim" id="provider-agent-name">{data.addMode ? `Adding: ${data.selectedAgent}` : `Configuring: ${data.selectedAgent}`}</p>
        <div id="provider-loading" className={`text-dim${data.providerLoading ? '' : ' hidden'}`}>Loading providers…</div>
        <div id="provider-add-choice" className={data.addMode ? '' : 'hidden'}>
          <h4>Carrier Provider</h4>
          <p id="provider-default-summary" className="text-dim">
            {data.carrierDefaultProvider?.reusable
              ? `Using Carrier default: ${data.carrierDefaultProvider.provider?.name || data.carrierDefaultProvider.id} (\`${data.carrierDefaultProvider.id}\`) · credential: ${data.carrierDefaultProvider.credential_backend || 'saved store'}.`
              : data.carrierDefaultProvider?.configured
                ? `Carrier default: ${data.carrierDefaultProvider.id}, but cannot reuse now.`
                : 'Carrier default provider is not configured.'}
          </p>
          <div className="btn-row">
            <button
              id="provider-use-default-continue"
              className={`btn-sm${data.carrierDefaultProvider?.reusable ? '' : ' hidden'}`}
              type="button"
              onClick={() => data.navigate(data.addMode ? '/install' : '/config')}
            >
              Use Carrier Provider (Recommended) →
            </button>
          </div>
        </div>
        <div id="provider-other-wrap" className={data.providers.length ? '' : 'hidden'}>
          <div id="provider-categories" className={data.providers.length ? '' : 'hidden'}>
            <div className="provider-category">
              <ul className="provider-list">
                {data.providers.map((provider) => (
                  <li
                    key={provider.id}
                    className={`provider-item${data.selectedProvider?.id === provider.id ? ' selected' : ''}`}
                    onClick={() => data.setSelectedProvider(provider)}
                  >
                    <strong>{provider.name}</strong> <code>{provider.id}</code>
                    {provider.example_model ? <br /> : null}
                    {provider.example_model ? <span className="text-dim">e.g. {provider.example_model}</span> : null}
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </div>
        <div id="provider-auth-section" className={`provider-auth-section${data.selectedProvider ? '' : ' hidden'}`} style={{ marginTop: '1rem' }}>
          <div id="provider-auth-label" className="text-dim">
            {data.selectedProvider?.auth_mode === 'api_key'
              ? `Paste API key for ${data.selectedProvider?.name} (${data.selectedProvider?.env_var || ''}):`
              : data.selectedProvider
                ? `${data.selectedProvider.name} requires external authentication.`
                : ''}
          </div>
          <input
            type="password"
            id="provider-api-key"
            placeholder="Paste API key here…"
            className={data.selectedProvider?.auth_mode === 'api_key' ? '' : 'hidden'}
            style={{ marginTop: '0.5rem' }}
            value={data.providerApiKey}
            onChange={(event) => data.setProviderApiKey(event.target.value)}
          />
          <div id="provider-auth-instructions" className={`text-dim${data.selectedProvider && data.selectedProvider.auth_mode !== 'api_key' ? '' : ' hidden'}`} style={{ marginTop: '0.5rem' }}>
            {data.selectedProvider && data.selectedProvider.auth_mode !== 'api_key'
              ? (data.addMode ? 'Paste access token below if you are not reusing Carrier credential.' : `Run: openclaw models auth login --provider ${data.selectedProvider.id}`)
              : ''}
          </div>
        </div>
        <div className="btn-row" style={{ marginTop: '1rem' }}>
          <button className="btn-secondary" id="provider-back" type="button" onClick={() => data.navigate(data.addMode ? '/setup' : '/agents')}>← Back</button>
          <button id="provider-skip" className="btn-secondary" type="button" onClick={() => data.navigate('/config')}>Skip →</button>
          <button id="provider-next" type="button" disabled={!data.providerNextEnabled} onClick={() => data.navigate(data.addMode ? '/install' : '/config')}>Continue →</button>
        </div>
        <div id="provider-msg">{data.providerMsg}</div>
      </div>
    </section>
  );
}
