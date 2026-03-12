import type { OnboardingData } from '../useOnboardingData';

export function WelcomeStep({ data }: { data: OnboardingData }) {
  return (
    <section id="view-welcome" className="view">
      <div className="card center-card">
        <h2>Welcome to Carrier</h2>
        <p className="text-dim">Detecting daemon connection…</p>
        <div id="welcome-status">{data.welcomeStatus}</div>
        <button
          id="welcome-continue"
          className={data.welcomeConnected ? '' : 'hidden'}
          type="button"
          onClick={() => data.navigate('/setup')}
        >
          Continue →
        </button>
      </div>
    </section>
  );
}
