import type { OnboardingData } from '../useOnboardingData';

export function CompleteStep({ data }: { data: OnboardingData }) {
  return (
    <section id="view-complete" className="view">
      <div className="card center-card">
        <h2 id="complete-title">✅ Setup Complete!</h2>
        <p className="text-dim">Your agent is being installed and started.</p>
        <p className="text-dim" id="complete-detail">
          {data.lastAddResult
            ? [
                data.lastAddResult.instanceId ? `Instance: ${data.lastAddResult.instanceId}` : '',
                data.lastAddResult.pairedChatId ? `Paired chat: ${data.lastAddResult.pairedChatId}` : '',
                data.lastAddResult.workspacePath ? `Workspace: ${data.lastAddResult.workspacePath}` : '',
                data.lastAddResult.configPath ? `Config: ${data.lastAddResult.configPath}` : '',
              ].filter(Boolean).join('\n')
            : ''}
        </p>
        <button id="complete-dashboard" type="button" onClick={data.finish}>
          Go to Dashboard →
        </button>
      </div>
    </section>
  );
}
