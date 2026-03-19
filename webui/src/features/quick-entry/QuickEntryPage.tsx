import type { ReactNode } from 'react';
import { RiArrowRightUpLine, RiCheckboxCircleLine, RiHome5Line, RiInboxArchiveLine, RiInformationLine, RiInstallLine, RiLoader4Line, RiPauseCircleLine, RiSparklingLine } from 'react-icons/ri';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { labelizeWorkValue } from '../work/model';
import { ChatComposer } from '../chat/components/ChatComposer';
import { ChatMessages } from '../chat/components/ChatMessages';
import { useQuickEntryData } from './useQuickEntryData';

function StatusRow({
  icon,
  title,
  meta,
  body,
  children,
}: {
  icon: ReactNode;
  title: string;
  meta: string;
  body: string;
  children?: ReactNode;
}) {
  return (
    <div className="rounded-[0.8rem] border border-[var(--color-line)] bg-white/92 px-3 py-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium text-[var(--color-ink)]">
            <span className="flex size-7 items-center justify-center rounded-md bg-[var(--color-panel)] text-[var(--color-primary)]">
              {icon}
            </span>
            <span className="truncate">{title}</span>
          </div>
          <div className="mt-1 text-[11px] uppercase tracking-[0.16em] text-[var(--color-ink-soft)]">{meta}</div>
          <div className="mt-2 text-sm leading-6 text-[var(--color-ink-soft)]">{body}</div>
        </div>
        {children ? <div className="flex shrink-0 items-center gap-1">{children}</div> : null}
      </div>
    </div>
  );
}

export function QuickEntryPage() {
  const data = useQuickEntryData();
  const recentProjects = data.chat.recentProjects.slice(0, 2);

  return (
    <section id="view-quick-entry" className="space-y-3">
      <Card className="rounded-[0.95rem] border-[var(--color-line)] bg-[rgba(251,250,246,0.96)] shadow-[0_12px_28px_rgba(45,41,36,0.05)]">
        <CardContent className="flex items-center justify-between gap-3 p-4">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="flex size-8 items-center justify-center rounded-md bg-[var(--color-panel)] text-[var(--color-primary)]">
                <RiInboxArchiveLine className="size-4" />
              </span>
              <h1 className="truncate text-sm font-medium text-[var(--color-ink)]">Carrier Inbox</h1>
            </div>
            <div className="mt-1 text-xs text-[var(--color-ink-soft)]">
              {data.approvalCount ? `${data.approvalCount} waiting for you` : 'Quiet until something needs you'}
            </div>
          </div>
          <TooltipProvider delayDuration={120}>
            <div className="flex items-center gap-1">
              {data.install.canInstall ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button type="button" variant="ghost" size="icon-sm" aria-label="Install Carrier Inbox" onClick={() => void data.install.promptInstall()}>
                      <RiInstallLine className="size-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent sideOffset={6} className="rounded-lg bg-[var(--color-ink)] px-3 py-1.5 text-xs text-[var(--color-panel)]">
                    Install Inbox
                  </TooltipContent>
                </Tooltip>
              ) : null}
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button type="button" variant="ghost" size="icon-sm" aria-label="Inbox help">
                    <RiInformationLine className="size-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent sideOffset={6} className="rounded-lg bg-[var(--color-ink)] px-3 py-1.5 text-xs text-[var(--color-panel)]">
                  Keep this window for quick replies. Open Home for the full workspace.
                </TooltipContent>
              </Tooltip>
              <Button asChild variant="outline" size="sm">
                <a href="/home" target="_blank" rel="noreferrer" aria-label="Open Home in browser">
                  <RiHome5Line className="size-4" />
                  Home
                </a>
              </Button>
            </div>
          </TooltipProvider>
        </CardContent>
      </Card>

      <div className="grid gap-3">
        <Card className="rounded-[0.95rem] border-[var(--color-line)] bg-[rgba(251,250,246,0.94)]">
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-sm font-medium">
              <RiPauseCircleLine className="size-4 text-[var(--color-primary)]" />
              Waiting
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {data.approvals.length ? data.approvals.map((execution) => (
              <StatusRow
                key={execution.id}
                icon={<RiCheckboxCircleLine className="size-4" />}
                title={String(execution.goal || execution.id || 'Approval needed')}
                meta={`${labelizeWorkValue(execution.status)} · ${String(execution.project || execution.team || 'carrier')}`}
                body={data.summarizeApproval(execution)}
              >
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={data.approveMutation.isPending || data.cancelMutation.isPending}
                  onClick={() => void data.approveExecution(String(execution.id))}
                >
                  Approve
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={data.approveMutation.isPending || data.cancelMutation.isPending}
                  onClick={() => void data.cancelExecution(String(execution.id))}
                >
                  Cancel
                </Button>
              </StatusRow>
            )) : (
              <div className="rounded-[0.8rem] border border-dashed border-[var(--color-line)] bg-[var(--color-panel)]/60 px-3 py-4 text-sm text-[var(--color-ink-soft)]">
                Nothing is waiting for approval right now.
              </div>
            )}

            {data.activeExecutions.length ? (
              <div className="space-y-3 border-t border-[var(--color-line)] pt-3">
                {data.activeExecutions.slice(0, 2).map((execution) => (
                  <StatusRow
                    key={execution.id}
                    icon={<RiLoader4Line className="size-4" />}
                    title={String(execution.goal || execution.id || 'Active task')}
                    meta={`${labelizeWorkValue(execution.status)} · ${String(execution.project || execution.team || 'carrier')}`}
                    body={data.summarizeActivity(execution)}
                  />
                ))}
              </div>
            ) : null}
          </CardContent>
        </Card>

        <Card className="rounded-[0.95rem] border-[var(--color-line)] bg-[rgba(251,250,246,0.94)]">
          <CardContent className="space-y-3 p-4">
            <div className="flex items-center justify-between gap-3">
              <div>
                <div className="flex items-center gap-2 text-sm font-medium text-[var(--color-ink)]">
                  <RiSparklingLine className="size-4 text-[var(--color-primary)]" />
                  Thread
                </div>
                <div className="mt-1 text-xs text-[var(--color-ink-soft)]">
                  {data.chat.activeSessionId ? `Session ${data.chat.activeSessionId}` : 'Fresh thread'}
                </div>
              </div>
              {recentProjects.length ? (
                <div className="flex items-center gap-1">
                  {recentProjects.map((project) => (
                    <span key={project.id} className="inline-flex h-6 items-center rounded-md bg-[var(--color-panel)] px-2 text-[11px] text-[var(--color-ink-soft)]">
                      {project.name}
                    </span>
                  ))}
                </div>
              ) : null}
            </div>
            <div className="rounded-[0.8rem] border border-[var(--color-line)] bg-white/92 px-3 py-3 text-sm leading-6 text-[var(--color-ink-soft)]">
              {data.latestMessage?.text || 'Start with a short instruction or a question.'}
            </div>
          </CardContent>
        </Card>

        <Card className="rounded-[0.95rem] border-[var(--color-line)] bg-[rgba(251,250,246,0.96)]">
          <CardContent className="space-y-3 p-3">
            <ChatMessages data={data.chat} className="h-[19rem] border-0 bg-transparent p-1" />
            <div className="border-t border-[var(--color-line)] pt-3">
              <ChatComposer data={data.chat} embedded />
            </div>
            <div className="flex justify-end">
              <Button asChild variant="ghost" size="sm">
                <a href="/home" target="_blank" rel="noreferrer" aria-label="Open full Home workspace">
                  <RiArrowRightUpLine className="size-4" />
                  Open Home
                </a>
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    </section>
  );
}
