import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';

export function WorkView({
  id,
  title,
  onRefresh,
  backTo,
  children,
}: {
  id: string;
  title: string;
  onRefresh?: () => void | Promise<void>;
  backTo?: string;
  children: ReactNode;
}) {
  return (
    <section id={id} className="view">
      <div className="section-head">
        <h2>{title}</h2>
        <div className="section-actions">
          {backTo ? <Link to={backTo} className="btn btn-secondary btn-sm">Back</Link> : null}
          {onRefresh ? (
            <button type="button" id={`${id}-refresh`} className="btn-sm btn-secondary" onClick={() => void onRefresh()}>
              Refresh
            </button>
          ) : null}
        </div>
      </div>
      {children}
    </section>
  );
}

export function WorkCard({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="card dashboard-stack">
      <div className="section-head">
        <h3>{title}</h3>
      </div>
      {children}
    </div>
  );
}

export function WorkList({ children }: { children: ReactNode }) {
  return (
    <ul style={{ listStyle: 'none', margin: 0, padding: 0, display: 'grid', gap: 12 }}>
      {children}
    </ul>
  );
}

export function WorkMetaList({ rows }: { rows: Array<{ label: string; value: ReactNode }> }) {
  const visibleRows = rows.filter((row) => row.value !== null && row.value !== undefined && row.value !== '');
  return (
    <dl style={{ display: 'grid', gap: 10, margin: 0 }}>
      {visibleRows.map((row) => (
        <div key={row.label} style={{ display: 'grid', gap: 4 }}>
          <dt className="execution-detail-title" style={{ marginBottom: 0 }}>{row.label}</dt>
          <dd className="execution-detail-line" style={{ margin: 0 }}>{row.value}</dd>
        </div>
      ))}
    </dl>
  );
}

export function WorkMessage({ text, tone = 'dim' }: { text: string; tone?: 'dim' | 'error' }) {
  return <p className={tone === 'error' ? 'msg-error' : 'text-dim'}>{text}</p>;
}
