import type { OnboardingData } from '../useOnboardingData';

export function InstallStep({ data }: { data: OnboardingData }) {
  const summaryLines = [`Agent: ${data.selectedAgent}`];
  if (data.addMode && data.channel) summaryLines.push(`Channel: ${data.channel}`);
  if (data.selectedProvider?.name) summaryLines.push(`Provider: ${data.selectedProvider.name}`);

  return (
    <section id="view-install" className="view">
      <div className="steps-indicator" id="steps-indicator-4"></div>
      <div className="card center-card">
        <h3>Step 5 — Confirm Installation</h3>
        <p id="install-summary">{summaryLines.join('\n')}</p>
        <div className="btn-row">
          <button className="btn-secondary" id="install-back" type="button" onClick={() => data.navigate(data.addMode ? '/provider' : '/config')}>← Back</button>
          <button id="install-confirm" type="button" onClick={() => void data.installAndStart()}>
            Install &amp; Start →
          </button>
        </div>
        <div id="install-msg">{data.installMsg}</div>
      </div>
    </section>
  );
}
