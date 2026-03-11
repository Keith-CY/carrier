import { usePoliciesData } from './usePoliciesData';
import { PolicyEditorCard } from './components/PolicyEditorCard';
import { PoliciesList } from './components/PoliciesList';
import { TriggerEditorCard } from './components/TriggerEditorCard';
import { TriggersList } from './components/TriggersList';

export function PoliciesSection() {
  const data = usePoliciesData();

  return (
    <section id="view-profiles" className="view">
      <div className="section-head">
        <h2 id="profiles-title">Policies</h2>
        <div className="section-actions">
          <button id="profiles-refresh" className="btn-sm btn-secondary" onClick={() => void data.refreshAll()}>Refresh</button>
        </div>
      </div>
      <div id="policies-shell">
        <PolicyEditorCard data={data} />
        <TriggerEditorCard data={data} />
        <div id="profiles-msg">
          {data.message.text ? <p className={`msg-${data.message.type}`}>{data.message.text}</p> : null}
        </div>
        <PoliciesList data={data} />
        <TriggersList data={data} />
      </div>
    </section>
  );
}
