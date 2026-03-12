import type { OnboardingData } from '../useOnboardingData';

export function AgentsStep({ data }: { data: OnboardingData }) {
  return (
    <section id="view-agents" className="view">
      <div className="steps-indicator" id="steps-indicator-2"></div>
      <div className="card">
        <h3>Step 2 — Select Agent</h3>
        <ul className="agent-select-list" id="agent-pick">
          {data.agents.map((agent) => {
            const id = String(agent?.id || agent?.ID || agent?.name || '').trim();
            return (
              <li
                key={id}
                className={data.selectedAgent === id ? 'selected' : ''}
                onClick={() => data.setSelectedAgent(id)}
              >
                {id}
              </li>
            );
          })}
        </ul>
        <div className="btn-row">
          <button className="btn-secondary" id="agents-back" type="button" onClick={() => data.navigate('/setup')}>← Back</button>
          <button id="agents-next" type="button" disabled={!data.selectedAgent} onClick={() => data.navigate('/provider')}>Continue →</button>
        </div>
        <div id="agents-msg">{data.agentsMsg}</div>
      </div>
    </section>
  );
}
