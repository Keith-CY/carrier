import type { OnboardingData } from '../useOnboardingData';

export function SetupStep({ data }: { data: OnboardingData }) {
  return (
    <section id="view-setup" className="view">
      <div className="steps-indicator" id="steps-indicator"></div>
      <div className="card">
        <h3 id="setup-title">{data.addMode ? `Step 1 — Choose Chat Channel for ${data.agentDisplayName(data.resolvedAddTargetAgent || '')}` : 'Step 1 — Configure Chat Channel'}</h3>
        <label htmlFor="provider" id="setup-provider-label">{data.addMode ? 'Channel' : 'Chat Channel'}</label>
        <select id="provider" value={data.channel} onChange={(event) => data.setChannel(event.target.value)}>
          <option value="">Select…</option>
          {data.channelOptions.map((option) => (
            <option key={option.id} value={option.id}>
              {option.displayName}
            </option>
          ))}
        </select>
        {data.selectedChannelStatus ? (
          <p id="setup-channel-summary" className="text-dim">
            {[
              data.channelRequiresPairing ? 'Requires Carrier pairing' : 'No pairing required',
              data.selectedChannelStatus.requiresWebhookSecret ? 'Webhook secret required' : 'Webhook secret optional',
              data.selectedChannelStatus.configured ? 'Already configured in gateway' : '',
            ].filter(Boolean).join(' · ')}
          </p>
        ) : null}
        <label htmlFor="provider-token" id="setup-token-label">{data.addMode ? 'Channel Bot Token' : 'Channel Bot Token'}</label>
        <input id="provider-token" type="password" placeholder="Bot token" value={data.channelToken} onChange={(event) => data.setChannelToken(event.target.value)} />
        <label htmlFor="webhook-secret">
          Webhook Secret <span className="text-dim">({data.channelRequiresWebhookSecret ? 'required' : 'optional'})</span>
        </label>
        <input id="webhook-secret" type="text" placeholder="Webhook verification secret" value={data.webhookSecret} onChange={(event) => data.setWebhookSecret(event.target.value)} />
        {data.channelRequiresPairing ? (
          <div id="setup-telegram-pair">
            <p className="text-dim" id="setup-pair-instruction">
              {data.channelChatId ? `Paired chat id: ${data.channelChatId}` : 'Click Start Pairing to get a code, then send it in your bot chat.'}
            </p>
            <div className="btn-row">
              <button
                id="setup-pair-use-carrier"
                type="button"
                className={`btn-sm${data.channelChatId ? '' : ' hidden'}`}
                onClick={() => {
                  if (data.channelChatId) data.setPairMsg(`Using Carrier paired ${data.selectedChannelStatus?.displayName || 'channel'} user: ${data.channelChatId}`);
                }}
              >
                Use Carrier paired user (Recommended)
              </button>
              <button id="setup-pair-start" type="button" className="btn-secondary btn-sm">Start Pairing</button>
            </div>
            <div id="setup-pair-msg">{data.pairMsg}</div>
          </div>
        ) : null}
        <div className="btn-row">
          <button
            id="setup-btn"
            type="button"
            disabled={
              !data.channel.trim() ||
              (data.channelRequiresBotToken && !data.channelToken.trim()) ||
              (data.channelRequiresWebhookSecret && !data.webhookSecret.trim()) ||
              (data.channelRequiresPairing && !data.channelChatId)
            }
            onClick={data.nextFromSetup}
          >
            Continue →
          </button>
        </div>
        <div id="setup-msg">{data.setupMsg}</div>
      </div>
    </section>
  );
}
