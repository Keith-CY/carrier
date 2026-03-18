import { Link } from 'react-router-dom';
import { RiArrowLeftLine, RiArrowRightUpLine, RiFolderOpenLine } from 'react-icons/ri';
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { InfoHint } from '@/components/ui/info-hint';
import { Skeleton } from '@/components/ui/skeleton';
import { formatDateTime } from '@/lib/format';
import { labelizeWorkValue } from './model';
import { useWorkProjectPageData } from './useWorkData';

export function ProjectDetailPage() {
  const data = useWorkProjectPageData();

  if (!data.enabled) {
    return (
      <Card className="rounded-[1.1rem] border-[color:rgba(20,24,31,0.08)] bg-white/82">
        <CardContent className="p-8">
          <Button asChild variant="outline">
            <Link to="/projects">Back to Projects</Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  if (data.query.isLoading || !data.query.data?.project) {
    return <Skeleton className="h-80 rounded-[1rem]" />;
  }

  const { project, items, runs } = data.query.data;

  return (
    <section id="view-project-detail" className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Button asChild variant="outline">
          <Link to="/projects">
            <RiArrowLeftLine className="size-4" />
            Back
          </Link>
        </Button>
        <Button asChild>
          <Link to={`/home?project=${encodeURIComponent(project.id)}`}>
            Run task in this project
            <RiArrowRightUpLine className="size-4" />
          </Link>
        </Button>
      </div>

      <Card className="rounded-[1.1rem] border-[color:rgba(20,24,31,0.08)] bg-[rgba(251,250,246,0.92)] shadow-[0_12px_32px_rgba(18,23,31,0.04)]">
        <CardContent className="grid gap-4 p-5 sm:grid-cols-[minmax(0,1fr)_15rem] sm:items-start sm:p-6">
          <div className="space-y-2">
            <div className="flex min-h-8 items-center gap-2">
              <span className="flex size-8 items-center justify-center rounded-lg border border-[color:rgba(20,24,31,0.08)] bg-[var(--color-panel)] text-[var(--color-primary)]">
                <RiFolderOpenLine className="size-4" />
              </span>
              <h1 className="text-xl leading-none font-semibold tracking-[0.01em] text-[var(--color-ink)]">{project.name}</h1>
              <InfoHint label="This page keeps only the project summary. Technical context stays in Advanced." />
            </div>
            <div className="text-sm leading-6 text-[var(--color-ink-soft)]">Tasks and recent runs.</div>
          </div>
          <div className="grid gap-3">
            <div className="min-h-[4.75rem] rounded-[0.95rem] border border-[color:rgba(20,24,31,0.08)] bg-white/90 p-4">
              <div className="text-[11px] uppercase tracking-[0.18em] text-[var(--color-ink-soft)]">State</div>
              <div className="mt-2 text-base font-medium text-[var(--color-ink)]">{labelizeWorkValue(project.state)}</div>
            </div>
            <div className="min-h-[4.75rem] rounded-[0.95rem] border border-[color:rgba(20,24,31,0.08)] bg-white/90 p-4">
              <div className="text-[11px] uppercase tracking-[0.18em] text-[var(--color-ink-soft)]">Last sync</div>
              <div className="mt-2 text-sm font-medium text-[var(--color-ink)]">{formatDateTime(project.lastSyncAt)}</div>
            </div>
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <Card className="rounded-[1rem] border-[color:rgba(20,24,31,0.08)] bg-white/90">
          <CardHeader>
            <CardTitle>Open tasks</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {items.length ? items.map((item) => (
              <div key={item.id} className="rounded-[0.9rem] border border-[color:rgba(20,24,31,0.08)] bg-[var(--color-panel)] px-4 py-3">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <div className="font-medium text-[var(--color-ink)]">{item.title}</div>
                    <div className="mt-1 text-sm leading-6 text-[var(--color-ink-soft)]">{item.description || 'No description yet.'}</div>
                  </div>
                  <div className="shrink-0 self-center text-right text-xs uppercase leading-none tracking-[0.14em] text-[var(--color-ink-soft)]">{labelizeWorkValue(item.state)}</div>
                </div>
              </div>
            )) : (
              <div className="rounded-[0.9rem] border border-dashed border-[color:rgba(20,24,31,0.12)] px-4 py-5 text-sm text-[var(--color-ink-soft)]">
                No items yet. Start from Home and let the base agent create the next useful task before coming back here.
              </div>
            )}
          </CardContent>
        </Card>

        <Card className="rounded-[1rem] border-[color:rgba(20,24,31,0.08)] bg-white/90">
          <CardHeader>
            <CardTitle>Latest runs</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {runs.length ? runs.map((run) => (
              <div key={run.id} className="rounded-[0.9rem] border border-[color:rgba(20,24,31,0.08)] bg-[var(--color-panel)] px-4 py-3">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <div className="font-medium text-[var(--color-ink)]">{run.executionId || run.id}</div>
                    <div className="mt-1 text-sm text-[var(--color-ink-soft)]">{labelizeWorkValue(run.phase)} · {labelizeWorkValue(run.verificationStatus)}</div>
                  </div>
                  <div className="text-xs text-[var(--color-ink-soft)]">{formatDateTime(run.updatedAt)}</div>
                </div>
              </div>
            )) : (
              <div className="rounded-[0.9rem] border border-dashed border-[color:rgba(20,24,31,0.12)] px-4 py-5 text-sm text-[var(--color-ink-soft)]">
                No runs have been recorded for this project yet.
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <Card className="rounded-[1rem] border-[color:rgba(20,24,31,0.08)] bg-white/90">
        <CardHeader>
          <CardTitle>Advanced</CardTitle>
        </CardHeader>
        <CardContent>
          <Accordion type="single" collapsible className="w-full">
            <AccordionItem value="technical">
              <AccordionTrigger>Technical context</AccordionTrigger>
              <AccordionContent className="space-y-2 text-sm text-[var(--color-ink-soft)]">
                <div>Source: {project.sourceType || 'n/a'}</div>
                <div>Source ref: {project.sourceRef || 'n/a'}</div>
                <div>Default branch: {project.defaultBranch || 'n/a'}</div>
                <div>Workflow path: {project.workflowPath || 'n/a'}</div>
                <div>Workflow digest: {project.workflowDigest || 'n/a'}</div>
                <div>Last sync error: {project.lastSyncError || 'none'}</div>
              </AccordionContent>
            </AccordionItem>
          </Accordion>
        </CardContent>
      </Card>
    </section>
  );
}
