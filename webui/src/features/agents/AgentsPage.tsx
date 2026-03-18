import { Link } from 'react-router-dom';
import { useMemo } from 'react';
import { RiArrowRightUpLine, RiRobot2Line } from 'react-icons/ri';
import { AnimatedNumber } from '@/components/motion-primitives/animated-number';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { InfoHint } from '@/components/ui/info-hint';
import { Skeleton } from '@/components/ui/skeleton';
import { useFeatures } from '@/app/useFeatures';
import { useDashboardInstancesData } from '../dashboard/useDashboardInstancesData';

function runtimeTone(value: unknown) {
  const runtime = String(value || '').trim().toLowerCase();
  if (runtime === 'running' || runtime === 'healthy') return 'ready';
  if (runtime === 'error' || runtime === 'failed') return 'attention';
  return 'idle';
}

export function AgentsPage() {
  const { featureFlags } = useFeatures();
  const data = useDashboardInstancesData();
  const agents = data.instances;

  const counts = useMemo(() => ({
    total: agents.length,
    ready: agents.filter((item: any) => runtimeTone(item?.runtime_state || item?.runtimeState || item?.runtime) === 'ready').length,
  }), [agents]);

  if (!featureFlags.remoteControlPlaneEnabled) {
    return (
      <Card className="rounded-[1.1rem] border-[color:rgba(20,24,31,0.08)] bg-white/82">
        <CardContent className="p-8 text-[var(--color-ink-soft)]">
          Agents become available when the remote control plane is enabled.
        </CardContent>
      </Card>
    );
  }

  return (
    <section id="view-agents" className="space-y-6">
      <Card className="rounded-[1.1rem] border-[color:rgba(20,24,31,0.08)] bg-[rgba(251,250,246,0.92)] shadow-[0_12px_32px_rgba(18,23,31,0.04)]">
        <CardContent className="grid gap-4 p-5 sm:grid-cols-[minmax(0,1fr)_15rem] sm:items-start sm:p-6">
          <div className="space-y-2">
            <div className="flex min-h-8 items-center gap-2">
              <span className="flex size-8 items-center justify-center rounded-lg border border-[color:rgba(20,24,31,0.08)] bg-[var(--color-panel)] text-[var(--color-primary)]">
                <RiRobot2Line className="size-4" />
              </span>
              <h1 className="text-xl leading-none font-semibold tracking-[0.01em] text-[var(--color-ink)]">Agents</h1>
              <InfoHint label="This list stays simple. Runtime and host details live inside each agent detail page." />
            </div>
            <div className="text-sm leading-6 text-[var(--color-ink-soft)]">Ready work surfaces.</div>
          </div>
          <div className="grid gap-3">
            <div className="min-h-[4.75rem] rounded-[0.95rem] border border-[color:rgba(20,24,31,0.08)] bg-white/90 p-4">
              <div className="text-[11px] uppercase tracking-[0.18em] text-[var(--color-ink-soft)]">Visible</div>
              <AnimatedNumber value={counts.total} className="mt-2 block text-2xl font-semibold text-[var(--color-ink)]" />
            </div>
            <div className="min-h-[4.75rem] rounded-[0.95rem] border border-[color:rgba(20,24,31,0.08)] bg-white/90 p-4">
              <div className="text-[11px] uppercase tracking-[0.18em] text-[var(--color-ink-soft)]">Ready</div>
              <AnimatedNumber value={counts.ready} className="mt-2 block text-2xl font-semibold text-[var(--color-ink)]" />
            </div>
          </div>
        </CardContent>
      </Card>

      {data.instancesQuery.isLoading ? (
        <div className="grid gap-4 xl:grid-cols-2">
          <Skeleton className="h-44 rounded-[1rem]" />
          <Skeleton className="h-44 rounded-[1rem]" />
        </div>
      ) : (
        <div className="grid gap-4 xl:grid-cols-2">
          {agents.map((item: any) => {
            const instanceId = String(item?.id || item?.ID || '').trim();
            const agentId = String(item?.agent_id || item?.agentID || item?.agent || item?.type || instanceId).trim();
            const runtime = String(item?.runtime_state || item?.runtimeState || item?.runtime || 'unknown').trim();
            return (
              <Card key={instanceId} className="rounded-[1rem] border-[color:rgba(20,24,31,0.08)] bg-white/90 shadow-[0_12px_32px_rgba(18,23,31,0.04)]">
                <CardHeader className="space-y-3">
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <CardTitle className="flex items-center gap-2 text-xl">
                        <RiRobot2Line className="size-5 text-[var(--color-primary)]" />
                        {agentId}
                      </CardTitle>
                      <div className="mt-2 text-xs leading-6 text-[var(--color-ink-soft)]">{instanceId}</div>
                    </div>
                    <div className="shrink-0 self-center text-right text-xs uppercase leading-none tracking-[0.16em] text-[var(--color-ink-soft)]">{runtime}</div>
                  </div>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="text-sm leading-6 text-[var(--color-ink-soft)]">
                    Channel {String(item?.channel || 'n/a')} · Provider {String(item?.provider || 'n/a')}
                  </div>
                  <div className="flex justify-end">
                    <Button asChild size="sm" variant="outline">
                      <Link to={`/agents/${encodeURIComponent(agentId)}`}>
                        Open detail
                        <RiArrowRightUpLine className="size-4" />
                      </Link>
                    </Button>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}
    </section>
  );
}
