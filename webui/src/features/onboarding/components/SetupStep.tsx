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
          <option value="telegram">Telegram</option>
          <option value="discord" disabled={data.addMode}>Discord</option>
          <option value="feishu" disabled={data.addMode}>Feishu</option>
        </select>
        <label htmlFor="provider-token" id="setup-token-label">{data.addMode ? 'Channel Bot Token' : 'Channel Bot Token'}</label>
        <input id="provider-token" type="password" placeholder="Bot token" value={data.channelToken} onChange={(event) => data.setChannelToken(event.target.value)} />
        <label htmlFor="webhook-secret">Webhook Secret <span className="text-dim">(optional)</span></label>
        <input id="webhook-secret" type="text" placeholder="Webhook verification secret" value={data.webhookSecret} onChange={(event) => data.setWebhookSecret(event.target.value)} />
        <div id="setup-telegram-pair" className={data.addMode ? '' : 'hidden'}>
          <p className="text-dim" id="setup-pair-instruction">
            {data.channelChatId ? `Paired chat id: ${data.channelChatId}` : 'Click Start Pairing to get a code, then send it in your Telegram bot chat.'}
          </p>
          <div className="btn-row">
            <button
              id="setup-pair-use-carrier"
              type="button"
              className={`btn-sm${data.channelChatId ? '' : ' hidden'}`}
              onClick={() => {
                if (data.channelChatId) data.setPairMsg(`Using Carrier paired Telegram user: ${data.channelChatId}`);
              }}
            >
              Use Carrier paired user (Recommended)
            </button>
            <button id="setup-pair-start" type="button" className="btn-secondary btn-sm">Start Pairing</button>
          </div>
          <div id="setup-pair-msg">{data.pairMsg}</div>
        </div>
        <div className="btn-row">
          <button
            id="setup-btn"
            type="button"
            disabled={!data.channel.trim() || !data.channelToken.trim() || (data.addMode && data.channel === 'telegram' && !data.channelChatId)}
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
