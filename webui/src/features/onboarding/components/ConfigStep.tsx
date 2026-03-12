import type { OnboardingData } from '../useOnboardingData';

export function ConfigStep({ data }: { data: OnboardingData }) {
  return (
    <section id="view-config" className="view">
      <div className="steps-indicator" id="steps-indicator-3"></div>
      <div className="card">
        <h3>Step 4 — Environment Variables</h3>
        <p className="text-dim" id="config-agent-name">{data.selectedAgent ? `Configuring: ${data.selectedAgent}` : ''}</p>
        <div id="env-fields">
          {data.envRows.map((row, index) => (
            <div key={`env-${index}`} className="env-row">
              <input
                type="text"
                placeholder="KEY"
                value={row.key}
                onChange={(event) => {
                  const next = data.envRows.slice();
                  next[index] = { ...next[index], key: event.target.value };
                  data.setEnvRows(next);
                }}
              />
              <input
                type="text"
                placeholder="VALUE"
                value={row.value}
                onChange={(event) => {
                  const next = data.envRows.slice();
                  next[index] = { ...next[index], value: event.target.value };
                  data.setEnvRows(next);
                }}
              />
            </div>
          ))}
        </div>
        <div className="btn-row">
          <button id="add-env" className="btn-secondary btn-sm" type="button" onClick={() => data.setEnvRows((current) => current.concat({ key: '', value: '' }))}>+ Add Env Var</button>
        </div>
        <div className="btn-row" style={{ marginTop: '1rem' }}>
          <button className="btn-secondary" id="config-back" type="button" onClick={() => data.navigate('/provider')}>← Back</button>
          <button id="config-next" type="button" onClick={() => data.navigate('/install')}>Continue →</button>
        </div>
        <div id="config-msg"></div>
      </div>
    </section>
  );
}
