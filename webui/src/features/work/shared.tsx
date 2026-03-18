import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { PageShell, type PageShellStat } from '../../app/page-shell';

export function WorkView({
  id,
  title,
  description,
  stats,
  onRefresh,
  backTo,
  children,
}: {
  id: string;
  title: string;
  description?: string;
  stats?: PageShellStat[];
  onRefresh?: () => void | Promise<void>;
  backTo?: string;
  children: ReactNode;
}) {
  return (
    <PageShell
      id={id}
      className="work-shell"
      eyebrow="Operate"
      title={title}
      description={description || 'Track projects, items, and execution runs with stronger portfolio-level hierarchy.'}
      stats={stats}
      actions={(
        <>
          {backTo ? <Link to={backTo} className="btn btn-secondary btn-sm">Back</Link> : null}
          {onRefresh ? (
            <button type="button" id={`${id}-refresh`} className="btn-sm btn-secondary" onClick={() => void onRefresh()}>
              Refresh
            </button>
          ) : null}
        </>
      )}
    >
      {children}
    </PageShell>
  );
}

export function WorkCard({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="card work-card">
      <div className="section-head">
        <h3>{title}</h3>
      </div>
      {children}
    </div>
  );
}

export function WorkList({ children }: { children: ReactNode }) {
  return (
    <ul className="work-list">
      {children}
    </ul>
  );
}

export function WorkMetaList({ rows }: { rows: Array<{ label: string; value: ReactNode }> }) {
  const visibleRows = rows.filter((row) => row.value !== null && row.value !== undefined && row.value !== '');
  return (
    <dl className="work-meta-list">
      {visibleRows.map((row) => (
        <div key={row.label} className="work-meta-list__row">
          <dt className="execution-detail-title">{row.label}</dt>
          <dd className="execution-detail-line">{row.value}</dd>
        </div>
      ))}
    </dl>
  );
}

export function WorkMessage({ text, tone = 'dim' }: { text: string; tone?: 'dim' | 'error' }) {
  return <p className={tone === 'error' ? 'msg-error' : 'text-dim'}>{text}</p>;
}
