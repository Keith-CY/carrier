import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { RiArrowRightUpLine, RiFolderOpenLine, RiSparklingLine } from 'react-icons/ri';
import { AnimatedNumber } from '@/components/motion-primitives/animated-number';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { InfoHint } from '@/components/ui/info-hint';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { formatDateTime } from '@/lib/format';
import { labelizeWorkValue } from './model';
import { useWorkPageData } from './useWorkData';

export function ProjectsPage() {
  const data = useWorkPageData();
  const [search, setSearch] = useState('');
  const projects = Array.isArray(data.query.data?.projects) ? data.query.data.projects : [];
  const items = Array.isArray(data.query.data?.items) ? data.query.data.items : [];
  const runs = Array.isArray(data.query.data?.runs) ? data.query.data.runs : [];

  const projectStats = useMemo(() => {
    const itemCounts = new Map<string, number>();
    const runCounts = new Map<string, number>();
    items.forEach((item) => itemCounts.set(item.projectId, (itemCounts.get(item.projectId) || 0) + 1));
    runs.forEach((run) => runCounts.set(run.projectId, (runCounts.get(run.projectId) || 0) + 1));
    return { itemCounts, runCounts };
  }, [items, runs]);

  const filteredProjects = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (!query) return projects;
    return projects.filter((project) => {
      const haystack = `${project.name} ${project.sourceRef} ${project.sourceType} ${project.state}`.toLowerCase();
      return haystack.includes(query);
    });
  }, [projects, search]);

  if (!data.enabled) {
    return (
      <Card className="rounded-[1rem] border-[var(--color-line)] bg-white/88">
        <CardContent className="space-y-4 p-6">
          <div className="flex items-center gap-2">
            <span className="flex size-8 items-center justify-center rounded-lg bg-[var(--color-panel-2)] text-[var(--color-primary)]">
              <RiSparklingLine className="size-4" />
            </span>
            <div className="text-base font-medium text-[var(--color-ink)]">Projects</div>
          </div>
          <div className="max-w-2xl text-sm leading-6 text-[var(--color-ink-soft)]">
            Projects appear when the remote control plane is available. Until then, start from Home and keep setup work inside Settings.
          </div>
          <Button asChild size="sm">
            <Link to="/home">Back to Home</Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  return (
    <section id="view-projects" className="space-y-6">
      <Card className="rounded-[1rem] border-[var(--color-line)] bg-[rgba(251,250,246,0.92)] shadow-[0_10px_30px_rgba(45,41,36,0.04)]">
        <CardContent className="grid gap-5 p-6 sm:grid-cols-[minmax(0,1fr)_18rem] sm:items-start sm:p-8">
          <div className="space-y-4">
            <div className="flex min-h-8 items-center gap-2">
              <span className="flex size-8 items-center justify-center rounded-lg bg-[var(--color-panel-2)] text-[var(--color-primary)]">
                <RiFolderOpenLine className="size-4" />
              </span>
              <h1 className="text-xl leading-none font-medium text-[var(--color-ink)]">Projects</h1>
              <InfoHint label="Projects stay lightweight. You can start from Home first and sort structure out later." />
            </div>
            <div className="max-w-md">
              <Input
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder="Search"
              />
            </div>
          </div>
          <div className="grid gap-3 sm:justify-self-end">
            <div className="min-h-[4.75rem] rounded-[0.85rem] border border-[var(--color-line)] bg-white/82 p-4">
              <div className="text-xs uppercase tracking-[0.2em] text-[var(--color-ink-soft)]">Projects</div>
              <AnimatedNumber value={filteredProjects.length} className="mt-2 block text-2xl font-medium text-[var(--color-ink)]" />
            </div>
            <div className="min-h-[4.75rem] rounded-[0.85rem] border border-[var(--color-line)] bg-white/82 p-4">
              <div className="text-xs uppercase tracking-[0.2em] text-[var(--color-ink-soft)]">Runs</div>
              <AnimatedNumber value={runs.length} className="mt-2 block text-2xl font-medium text-[var(--color-ink)]" />
            </div>
          </div>
        </CardContent>
      </Card>

      {data.query.isLoading ? (
        <div className="grid gap-4 xl:grid-cols-2">
          <Skeleton className="h-48 rounded-[1rem]" />
          <Skeleton className="h-48 rounded-[1rem]" />
        </div>
      ) : (
        <div className="grid gap-4 xl:grid-cols-2">
          {filteredProjects.map((project) => (
            <Card key={project.id} className="rounded-[1rem] border-[var(--color-line)] bg-white/88 shadow-[0_8px_20px_rgba(45,41,36,0.04)]">
              <CardHeader className="space-y-3">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <CardTitle className="text-lg">{project.name}</CardTitle>
                    <div className="mt-1 text-xs leading-6 text-[var(--color-ink-soft)]">{project.sourceRef || 'No source reference yet'}</div>
                  </div>
                  <span className="shrink-0 self-center text-right text-xs leading-none text-[var(--color-ink-soft)]">{labelizeWorkValue(project.state)}</span>
                </div>
              </CardHeader>
              <CardContent className="space-y-5">
                <div className="grid gap-3 sm:grid-cols-3">
                  <div className="rounded-[0.8rem] border border-[var(--color-line)] bg-[var(--color-panel)] p-3">
                    <div className="text-xs uppercase tracking-[0.2em] text-[var(--color-ink-soft)]">Items</div>
                    <div className="mt-2 text-xl font-medium text-[var(--color-ink)]">{projectStats.itemCounts.get(project.id) || 0}</div>
                  </div>
                  <div className="rounded-[0.8rem] border border-[var(--color-line)] bg-[var(--color-panel)] p-3">
                    <div className="text-xs uppercase tracking-[0.2em] text-[var(--color-ink-soft)]">Runs</div>
                    <div className="mt-2 text-xl font-medium text-[var(--color-ink)]">{projectStats.runCounts.get(project.id) || 0}</div>
                  </div>
                  <div className="rounded-[0.8rem] border border-[var(--color-line)] bg-[var(--color-panel)] p-3">
                    <div className="text-xs uppercase tracking-[0.2em] text-[var(--color-ink-soft)]">Branch</div>
                    <div className="mt-2 text-sm font-medium text-[var(--color-ink)]">{project.defaultBranch || 'n/a'}</div>
                  </div>
                </div>
                <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-[var(--color-ink-soft)]">
                  <span>{project.sourceType || 'manual'} · last sync {formatDateTime(project.lastSyncAt)}</span>
                  <Button asChild size="sm">
                    <Link to={`/projects/${encodeURIComponent(project.id)}`}>
                      Open
                      <RiArrowRightUpLine className="size-4" />
                    </Link>
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </section>
  );
}
