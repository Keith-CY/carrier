import { RiAddLine, RiArrowRightLine, RiRefreshLine, RiSettings4Line, RiStopCircleLine } from 'react-icons/ri';
import { Button } from '@/components/ui/button';
import { InfoHint } from '@/components/ui/info-hint';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet';
import { Textarea } from '@/components/ui/textarea';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import type { ChatData } from '../useChatData';

export function ChatComposer({ data, embedded = false }: { data: ChatData; embedded?: boolean }) {
  return (
    <div className={cn(
      'rounded-[1.6rem] border border-[color:rgba(20,24,31,0.08)] bg-white/88 p-4 shadow-[0_22px_60px_rgba(18,23,31,0.08)]',
      embedded ? 'rounded-[1.4rem] border-white/0 bg-transparent p-0 shadow-none' : '',
    )}>
      <div className="mb-3 flex items-center justify-between gap-3">
        {embedded ? <div className="text-sm text-[var(--color-ink-soft)]">{data.activeSessionId ? `Session ${data.activeSessionId}` : 'Base agent'}</div> : (
          <div>
            <div className="text-sm font-semibold text-[var(--color-ink)]">Ask the base agent</div>
            <div className="text-sm text-[var(--color-ink-soft)]">{data.statusText}</div>
          </div>
        )}
        <TooltipProvider delayDuration={120}>
          <div className="flex items-center gap-1">
            {embedded ? <InfoHint label="Press Enter to send. Use the small controls here only when you need a new thread, retry, or advanced context." /> : null}
          <Sheet open={data.advancedOpen} onOpenChange={data.setAdvancedOpen}>
            <SheetTrigger asChild>
              <Button variant={embedded ? 'ghost' : 'outline'} size="icon-sm" aria-label="Open advanced chat settings">
                <RiSettings4Line className="size-4" />
              </Button>
            </SheetTrigger>
            <SheetContent className="w-full sm:max-w-md">
              <SheetHeader>
                <SheetTitle>Advanced chat context</SheetTitle>
              </SheetHeader>
              <div className="mt-6 space-y-5">
                <div className="space-y-2">
                  <Label htmlFor="provider-override">Provider override</Label>
                  <Input
                    id="provider-override"
                    placeholder="Leave blank to use webui routing"
                    value={data.providerOverride}
                    onChange={(event) => data.setProviderOverride(event.target.value)}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="project-context">Project context</Label>
                  <Select value={data.selectedProjectId || 'none'} onValueChange={(value) => data.setSelectedProjectId(value === 'none' ? '' : value)}>
                    <SelectTrigger id="project-context">
                      <SelectValue placeholder="No project context" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">No project context</SelectItem>
                      {data.projectOptions.map((project) => (
                        <SelectItem key={project.id} value={project.id}>
                          {project.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="rounded-2xl border border-dashed border-[color:rgba(20,24,31,0.12)] bg-[var(--color-panel-2)]/55 p-4 text-sm text-[var(--color-ink-soft)]">
                  Keep advanced choices here. The main composer stays focused on the shortest path: tell the agent what you need, then refine only if necessary.
                </div>
              </div>
            </SheetContent>
          </Sheet>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size={embedded ? 'icon-sm' : 'sm'} aria-label="Start a new thread" onClick={data.clearConversation}>
                <RiAddLine className="size-4" />
                {embedded ? null : 'New thread'}
              </Button>
            </TooltipTrigger>
            {embedded ? <TooltipContent sideOffset={6} className="rounded-lg bg-[var(--color-ink)] px-3 py-1.5 text-xs text-[var(--color-panel)]">New thread</TooltipContent> : null}
          </Tooltip>
          </div>
        </TooltipProvider>
      </div>

      <div className="space-y-3">
        <Textarea
          id="chat-input"
          placeholder="Start a task, ask what is blocked, or ask what Carrier can do next."
          className={cn(
            'min-h-28 resize-none rounded-[1.25rem] border-[color:rgba(20,24,31,0.08)] bg-white/70 px-4 py-3 text-base shadow-none focus-visible:ring-[var(--color-primary)]/30',
            embedded ? 'bg-white/84' : '',
          )}
          value={data.input}
          onChange={(event) => data.setInput(event.target.value)}
          onKeyDown={data.onKeyDown}
        />

        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="text-xs text-[var(--color-ink-soft)]">
            <span className="rounded-md bg-[var(--color-panel-2)] px-2 py-1 font-mono">Enter</span>
            <span className="mx-1">send</span>
            <span className="rounded-md bg-[var(--color-panel-2)] px-2 py-1 font-mono">Shift+Enter</span>
            <span className="ml-1">line break</span>
          </div>
          <TooltipProvider delayDuration={120}>
            <div className="flex items-center gap-1">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="ghost" size={embedded ? 'icon-sm' : 'sm'} aria-label="Retry last message" onClick={data.retryLast}>
                  <RiRefreshLine className="size-4" />
                  {embedded ? null : 'Retry'}
                </Button>
              </TooltipTrigger>
              {embedded ? <TooltipContent sideOffset={6} className="rounded-lg bg-[var(--color-ink)] px-3 py-1.5 text-xs text-[var(--color-panel)]">Retry</TooltipContent> : null}
            </Tooltip>
            {data.isStreaming ? (
              <Button variant="outline" size={embedded ? 'icon-sm' : 'sm'} aria-label="Stop and reset" onClick={data.clearConversation}>
                <RiStopCircleLine className="size-4" />
                {embedded ? null : 'Stop'}
              </Button>
            ) : (
              <Button id="chat-send" type="button" size={embedded ? 'icon-sm' : 'sm'} aria-label="Send message" onClick={() => void data.send()}>
                {embedded ? null : 'Send'}
                <RiArrowRightLine className="size-4" />
              </Button>
            )}
            </div>
          </TooltipProvider>
        </div>
      </div>
    </div>
  );
}
