export function TriggerBlock({ execution }: { execution: any }) {
  const triggerSource = String(execution?.triggerSource || '').trim();
  const triggerID = String(execution?.triggerId || '').trim();
  const triggerEvent = String(execution?.triggerEvent || '').trim();
  const initiator = String(execution?.initiator || '').trim();

  if (!(triggerSource || triggerID || triggerEvent || initiator)) return null;

  return (
    <div className="execution-detail-block">
      <div className="execution-detail-title">Trigger</div>
      {triggerSource ? <div className="execution-detail-line">source: {triggerSource}</div> : null}
      {triggerID ? <div className="execution-detail-line">id: {triggerID}</div> : null}
      {triggerEvent ? <div className="execution-detail-line">event: {triggerEvent}</div> : null}
      {initiator ? <div className="execution-detail-line">initiator: {initiator}</div> : null}
    </div>
  );
}
