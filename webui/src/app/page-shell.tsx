import type { ReactNode } from 'react';

export type PageShellStat = {
  label: string;
  value: ReactNode;
  detail?: ReactNode;
};

export function PageShell({
  id,
  className,
  eyebrow,
  title,
  titleId,
  description,
  actions,
  stats,
  children,
}: {
  id: string;
  className?: string;
  eyebrow?: ReactNode;
  title: ReactNode;
  titleId?: string;
  description?: ReactNode;
  actions?: ReactNode;
  stats?: PageShellStat[];
  children: ReactNode;
}) {
  const visibleStats = (stats || []).filter((stat) => stat.value !== null && stat.value !== undefined && stat.value !== '');

  return (
    <section id={id} className={['view', 'page-shell', className || ''].filter(Boolean).join(' ')}>
      <div className="page-shell__hero">
        <div className="page-shell__header">
          <div className="page-shell__copy">
            {eyebrow ? <div className="page-shell__eyebrow">{eyebrow}</div> : null}
            <h1 id={titleId} className="page-shell__title">{title}</h1>
            {description ? <p className="page-shell__description">{description}</p> : null}
          </div>
          {actions ? <div className="page-shell__actions">{actions}</div> : null}
        </div>
        {visibleStats.length ? (
          <div className="page-shell__stats">
            {visibleStats.map((stat) => (
              <div key={stat.label} className="page-shell__stat">
                <div className="page-shell__stat-label">{stat.label}</div>
                <div className="page-shell__stat-value">{stat.value}</div>
                {stat.detail ? <div className="page-shell__stat-detail">{stat.detail}</div> : null}
              </div>
            ))}
          </div>
        ) : null}
      </div>
      <div className="page-shell__body">
        {children}
      </div>
    </section>
  );
}
