import { PageShell } from '../../app/page-shell';
import { usePoliciesData } from './usePoliciesData';
import { PolicyEditorCard } from './components/PolicyEditorCard';
import { PoliciesList } from './components/PoliciesList';
import { TriggerEditorCard } from './components/TriggerEditorCard';
import { TriggersList } from './components/TriggersList';

export function PoliciesSection() {
  const data = usePoliciesData();

  return (
    <PageShell
      id="view-profiles"
      eyebrow="Configure"
      title="Policies"
      titleId="profiles-title"
      description="Set the operating guardrails, escalation behaviors, and automatic triggers that shape how Carrier behaves."
      actions={(
        <button id="profiles-refresh" className="btn-sm btn-secondary" onClick={() => void data.refreshAll()}>Refresh</button>
      )}
    >
      <div id="policies-shell">
        <PolicyEditorCard data={data} />
        <TriggerEditorCard data={data} />
        <div id="profiles-msg">
          {data.message.text ? <p className={`msg-${data.message.type}`}>{data.message.text}</p> : null}
        </div>
        <PoliciesList data={data} />
        <TriggersList data={data} />
      </div>
    </PageShell>
  );
}
