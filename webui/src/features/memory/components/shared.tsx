export function renderMemorySection(items: { title: string; meta: string[] }[], empty: string) {
  if (!items.length) return <div className="text-dim">{empty}</div>;
  return items.map((item) => (
    <div key={`${item.title}-${item.meta.join('|')}`} className="agent-card memory-card">
      <strong>{item.title}</strong>
      {item.meta.map((line) => (
        <div key={line} className="execution-detail-line">{line}</div>
      ))}
    </div>
  ));
}
