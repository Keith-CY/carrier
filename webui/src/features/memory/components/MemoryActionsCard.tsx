import type { MemoryData } from '../useMemoryData';

export function MemoryActionsCard({ data }: { data: MemoryData }) {
  const {
    canMutate,
    instanceId,
    setInstanceId,
    instanceScope,
    setInstanceScope,
    distillReason,
    setDistillReason,
    distillDryRun,
    setDistillDryRun,
    actionMessage,
    runAction,
  } = data;

  return (
    <div className="card dashboard-stack">
      <div className="section-head">
        <div>
          <h3>Instance Actions</h3>
          <p className="text-dim">Attach or detach a scope, then distill learnings back into base memory.</p>
        </div>
      </div>
      <div className="form-grid">
        <div>
          <label htmlFor="memory-instance-id">Instance ID</label>
          <input id="memory-instance-id" type="text" placeholder="picoclaw-main" value={instanceId} onChange={(event) => setInstanceId(event.target.value)} />
        </div>
        <div>
          <label htmlFor="memory-instance-scope">Scope</label>
          <input id="memory-instance-scope" type="text" placeholder="shared:profile" value={instanceScope} onChange={(event) => setInstanceScope(event.target.value)} />
        </div>
        <div>
          <label htmlFor="memory-distill-reason">Distill Reason</label>
          <input id="memory-distill-reason" type="text" placeholder="promote learnings" value={distillReason} onChange={(event) => setDistillReason(event.target.value)} />
        </div>
        <div>
          <label htmlFor="memory-distill-dry-run">Distill Dry Run</label>
          <input id="memory-distill-dry-run" type="checkbox" checked={distillDryRun} onChange={(event) => setDistillDryRun(event.target.checked)} />
        </div>
      </div>
      <div className="btn-row">
        <button id="memory-attach" className="btn-sm" disabled={!canMutate} onClick={() => runAction('attach')}>Attach</button>
        <button id="memory-detach" className="btn-sm btn-secondary" disabled={!canMutate} onClick={() => runAction('detach')}>Detach</button>
        <button id="memory-distill" className="btn-sm" disabled={!canMutate} onClick={() => runAction('distill')}>Distill</button>
      </div>
      <div id="memory-action-msg">{actionMessage.text ? <p className={`msg-${actionMessage.type}`}>{actionMessage.text}</p> : null}</div>
    </div>
  );
}
