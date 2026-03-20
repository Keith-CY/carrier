export function ExecutionMetadataBlock({ execution, metadata }: { execution: any; metadata?: any }) {
  const effectiveMetadata = metadata && typeof metadata === 'object'
    ? metadata
    : {
        sharedInstructions: execution?.sharedInstructions,
        runtimeContextManifest: execution?.runtimeContextManifest,
        guardrails: execution?.guardrails,
      };
  const sharedInstructions = Array.isArray(effectiveMetadata?.sharedInstructions) ? effectiveMetadata.sharedInstructions : [];
  const runtimeContextEntries = Array.isArray(effectiveMetadata?.runtimeContextManifest?.entries) ? effectiveMetadata.runtimeContextManifest.entries : [];
  const guardrails = effectiveMetadata?.guardrails && typeof effectiveMetadata.guardrails === 'object' ? effectiveMetadata.guardrails : {};
  const guardrailEvents = Array.isArray(guardrails?.events) ? guardrails.events : [];
  const graphNodes = Array.isArray(effectiveMetadata?.nodes) ? effectiveMetadata.nodes : [];
  const graphEdges = Array.isArray(effectiveMetadata?.edges) ? effectiveMetadata.edges : [];

  if (!sharedInstructions.length && !runtimeContextEntries.length && !guardrailEvents.length && !graphNodes.length && !graphEdges.length) return null;

  return (
    <div className="execution-detail-block">
      <div className="execution-detail-title">Execution Metadata</div>
      {sharedInstructions.length ? <div className="execution-detail-line">Shared instructions</div> : null}
      {sharedInstructions.map((item: any, index: number) => (
        <div key={`instruction-${String(item?.id || item?.title || index)}`} className="execution-detail-line">
          {String(item?.title || item?.id || `instruction-${index + 1}`).trim() || `instruction-${index + 1}`}: {String(item?.content || '').trim()}
        </div>
      ))}
      {runtimeContextEntries.length ? <div className="execution-detail-line">Runtime context manifest</div> : null}
      {runtimeContextEntries.map((item: any, index: number) => (
        <div key={`runtime-context-${String(item?.key || index)}`} className="execution-detail-line">
          {String(item?.key || `entry-${index + 1}`).trim()}
          {String(item?.class || '').trim() ? ` · class=${String(item.class).trim()}` : ''}
          {String(item?.source || '').trim() ? ` · source=${String(item.source).trim()}` : ''}
          {String(item?.valueType || '').trim() ? ` · type=${String(item.valueType).trim()}` : ''}
          {String(item?.digest || '').trim() ? ` · digest=${String(item.digest).trim()}` : ''}
        </div>
      ))}
      {guardrails?.summary ? (
        <div className="execution-detail-line">
          Guardrails: total={Number(guardrails.summary.total || 0)} · allow={Number(guardrails.summary.allowCount || 0)} · ask={Number(guardrails.summary.askCount || 0)} · deny={Number(guardrails.summary.denyCount || 0)}
        </div>
      ) : null}
      {guardrailEvents.map((item: any, index: number) => (
        <div key={`guardrail-${index}`} className="execution-detail-line">
          {String(item?.scope || 'unknown').trim()} · {String(item?.decision || 'unknown').trim()}
          {String(item?.ruleId || '').trim() ? ` · rule=${String(item.ruleId).trim()}` : ''}
          {String(item?.resolution || '').trim() ? ` · resolution=${String(item.resolution).trim()}` : ''}
          {String(item?.reason || '').trim() ? ` · ${String(item.reason).trim()}` : ''}
        </div>
      ))}
      {(graphNodes.length || graphEdges.length) ? (
        <div className="execution-detail-line">Graph: nodes={graphNodes.length} · edges={graphEdges.length}</div>
      ) : null}
      <pre className="code-block">{JSON.stringify(effectiveMetadata, null, 2)}</pre>
    </div>
  );
}
