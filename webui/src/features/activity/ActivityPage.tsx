import { Link } from 'react-router-dom';
import { RiArrowRightUpLine, RiPulseLine } from 'react-icons/ri';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { InfoHint } from '@/components/ui/info-hint';
import { Input } from '@/components/ui/input';
import { executionCounts } from '../executions/ExecutionDetailContent';
import { useExecutionsData } from '../executions/useExecutionsData';
import { labelizeWorkValue } from '../work/model';
import { useLogsData } from '../logs/useLogsData';
import { useMemoryData } from '../memory/useMemoryData';

export function ActivityPage() {
  const executions = useExecutionsData();
  const logs = useLogsData();
  const memory = useMemoryData();
  const selectedExecution = executions.selectedExecution || executions.filteredExecutions[0] || null;
  const selectedCounts = selectedExecution ? executionCounts(selectedExecution) : null;

  return (
    <section id="view-activity" className="space-y-6">
      <Card className="rounded-[1rem] border-[var(--color-line)] bg-[rgba(251,250,246,0.92)] shadow-[0_10px_30px_rgba(45,41,36,0.04)]">
        <CardContent className="flex items-center justify-between gap-3 p-5 sm:p-6">
          <div className="flex min-h-8 items-center gap-2">
            <span className="flex size-8 items-center justify-center rounded-lg bg-[var(--color-panel-2)] text-[var(--color-primary)]">
              <RiPulseLine className="size-4" />
            </span>
            <h1 className="text-xl leading-none font-medium text-[var(--color-ink)]">Activity</h1>
            <InfoHint label="Use Activity only when you need the full trail: runs, logs, approvals, or memory lookups." />
          </div>
        </CardContent>
      </Card>

      <Tabs defaultValue="runs" className="space-y-4">
        <TabsList className="grid w-full max-w-xl grid-cols-3 rounded-lg bg-white/82 p-1">
          <TabsTrigger value="runs" className="rounded-md">Runs</TabsTrigger>
          <TabsTrigger value="logs" className="rounded-md">Logs</TabsTrigger>
          <TabsTrigger value="knowledge" className="rounded-md">Knowledge</TabsTrigger>
        </TabsList>

        <TabsContent value="runs" className="grid gap-4 xl:grid-cols-[minmax(0,0.95fr)_minmax(0,1.05fr)]">
          <Card className="rounded-[1rem] border-[var(--color-line)] bg-white/88">
            <CardHeader className="space-y-3">
              <CardTitle>Runs</CardTitle>
              <div className="grid gap-3 sm:grid-cols-3">
                <Input
                  value={executions.searchValue}
                  onChange={(event) => executions.setParam('search', event.target.value)}
                  placeholder="Search runs"
                />
              </div>
            </CardHeader>
            <CardContent className="space-y-3">
              {executions.filteredExecutions.map((execution) => (
                <button
                  key={String(execution?.id || '')}
                  type="button"
                  onClick={() => executions.setSelectedExecutionId(String(execution?.id || ''))}
                  className="w-full rounded-[0.8rem] border border-[var(--color-line)] bg-[var(--color-panel)] px-4 py-3 text-left transition hover:border-[var(--color-line-strong)]"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <div className="font-medium text-[var(--color-ink)]">{String(execution?.goal || execution?.id || 'Execution')}</div>
                      <div className="mt-1 text-sm text-[var(--color-ink-soft)]">
                        {String(execution?.updatedAt || execution?.createdAt || 'n/a')}
                      </div>
                    </div>
                    <span className="shrink-0 self-center text-right text-xs leading-none text-[var(--color-ink-soft)]">{labelizeWorkValue(execution?.status)}</span>
                  </div>
                </button>
              ))}
            </CardContent>
          </Card>

          <Card className="rounded-[1rem] border-[var(--color-line)] bg-white/88">
            <CardHeader className="space-y-3">
              <CardTitle>Selected run</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              {selectedExecution ? (
                <>
                  <div>
                    <div className="text-2xl font-semibold text-[var(--color-ink)]">{String(selectedExecution?.goal || selectedExecution?.id || 'Execution')}</div>
                    <div className="mt-2 text-sm leading-7 text-[var(--color-ink-soft)]">{String(selectedExecution?.id || '')}</div>
                  </div>
                  <div className="grid gap-3 sm:grid-cols-3">
                    <div className="min-h-[4.75rem] rounded-[0.8rem] border border-[var(--color-line)] bg-[var(--color-panel)] p-3">
                      <div className="text-xs uppercase tracking-[0.2em] text-[var(--color-ink-soft)]">Status</div>
                      <div className="mt-2 text-sm font-semibold text-[var(--color-ink)]">{labelizeWorkValue(selectedExecution?.status)}</div>
                    </div>
                    <div className="min-h-[4.75rem] rounded-[0.8rem] border border-[var(--color-line)] bg-[var(--color-panel)] p-3">
                      <div className="text-xs uppercase tracking-[0.2em] text-[var(--color-ink-soft)]">Completed</div>
                      <div className="mt-2 text-sm font-semibold text-[var(--color-ink)]">{selectedCounts?.completed || 0}</div>
                    </div>
                    <div className="min-h-[4.75rem] rounded-[0.8rem] border border-[var(--color-line)] bg-[var(--color-panel)] p-3">
                      <div className="text-xs uppercase tracking-[0.2em] text-[var(--color-ink-soft)]">Failed</div>
                      <div className="mt-2 text-sm font-semibold text-[var(--color-ink)]">{selectedCounts?.failed || 0}</div>
                    </div>
                  </div>
                  {executions.selectedPolicyAskPending ? (
                    <div className="rounded-[1.2rem] border border-[color:rgba(183,117,24,0.24)] bg-[rgba(255,245,227,0.9)] p-4 text-sm text-[var(--color-ink)]">
                      This run is waiting for policy approval.
                    </div>
                  ) : null}
                </>
              ) : (
                <div className="text-sm text-[var(--color-ink-soft)]">No execution selected yet.</div>
              )}
              <div className="flex justify-end">
                <Button asChild variant="outline" size="sm">
                  <Link to="/home">
                    Back to Home
                    <RiArrowRightUpLine className="size-4" />
                  </Link>
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="logs">
          <Card className="rounded-[1rem] border-[var(--color-line)] bg-white/88">
            <CardHeader className="flex flex-row items-center justify-between gap-3">
              <div>
                <CardTitle>Logs</CardTitle>
                <div className="mt-2 text-sm text-[var(--color-ink-soft)]">{logs.statusText}</div>
              </div>
              <div className="flex gap-2">
                <Button variant="outline" size="sm" onClick={() => logs.connect()}>Connect</Button>
                <Button variant="outline" size="sm" onClick={() => logs.togglePause()}>{logs.paused ? 'Resume' : 'Pause'}</Button>
                <Button variant="ghost" size="sm" onClick={() => logs.clear()}>Clear</Button>
              </div>
            </CardHeader>
            <CardContent className="space-y-3">
              <Input value={logs.searchQuery} onChange={(event) => logs.setSearchQuery(event.target.value)} placeholder="Filter logs" />
              <div className="space-y-2">
                {logs.visibleEntries.slice(-60).map((entry) => (
                  <div key={entry.id} className="rounded-[0.8rem] border border-[var(--color-line)] bg-[var(--color-panel)] px-4 py-3 font-mono text-xs leading-6 text-[var(--color-ink)]">
                    [{entry.timestamp}] {entry.level} {entry.message}
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="knowledge">
          <Card className="rounded-[1rem] border-[var(--color-line)] bg-white/88">
            <CardHeader className="space-y-3">
              <CardTitle>Knowledge search</CardTitle>
              <div className="text-sm text-[var(--color-ink-soft)]">
                Search memory only when you need historical evidence or a distilled answer.
              </div>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="grid gap-3 sm:grid-cols-[12rem_minmax(0,1fr)_10rem]">
                <Input value={memory.subject} onChange={(event) => memory.setSubject(event.target.value)} placeholder="Subject" />
                <Input value={memory.searchQuery} onChange={(event) => memory.setSearchQuery(event.target.value)} placeholder="Ask memory" />
                <Button size="sm" onClick={() => memory.runSearch()}>Search</Button>
              </div>
              <div className="text-sm text-[var(--color-ink-soft)]">{memory.message.text}</div>
              <div className="space-y-2">
                {memory.searchResults.map((result: any, index: number) => (
                  <div key={`${result?.id || 'memory'}-${index}`} className="rounded-[0.8rem] border border-[var(--color-line)] bg-[var(--color-panel)] px-4 py-3 text-sm leading-7 text-[var(--color-ink)]">
                    {String(result?.text || result?.summary || JSON.stringify(result))}
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </section>
  );
}
