import { Link } from 'react-router-dom';
import { RiArrowRightUpLine, RiFolderOpenLine, RiInformationLine, RiLoader4Line, RiPulseLine, RiSettings3Line, RiSparklingLine } from 'react-icons/ri';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { InfoHint } from '@/components/ui/info-hint';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import type { ChatData } from './useChatData';
import { ChatComposer } from './components/ChatComposer';
import { ChatMessages } from './components/ChatMessages';

export function ChatSection({ data }: { data: ChatData }) {
  const starterPrompts = Array.isArray(data?.starterPrompts) ? data.starterPrompts : [
    { label: 'Start a task', prompt: '' },
    { label: 'What can you do?', prompt: '' },
    { label: 'Show my active work', prompt: '' },
    { label: 'Continue last task', prompt: '' },
  ];
  const recentProjects = Array.isArray(data?.recentProjects) ? data.recentProjects : [];
  const statusText = data?.statusText || 'Base agent ready.';
  const systemSummary = data?.systemSummary || 'Connect a provider and start from Home. Detailed setup remains tucked away until you need it.';
  const primaryPrompt = starterPrompts[0];
  const secondaryPrompts = starterPrompts.slice(1, 3);
  const currentProject = recentProjects.find((project) => project.id === data?.selectedProjectId);

  return (
    <section id="view-home" className="space-y-4">
      <Card className="overflow-hidden rounded-[1rem] border-[var(--color-line)] bg-[rgba(251,250,246,0.92)] shadow-[0_12px_30px_rgba(45,41,36,0.05)]">
        <CardContent className="p-0">
          <div className="flex items-center justify-between gap-3 border-b border-[var(--color-line)] px-4 py-3">
            <div className="flex min-w-0 items-center gap-2">
              <span className="flex size-8 items-center justify-center rounded-lg bg-[var(--color-panel-2)] text-[var(--color-primary)]">
                <RiSparklingLine className="size-4" />
              </span>
              <h1 className="text-base leading-none font-medium text-[var(--color-ink)]">Home</h1>
              {currentProject ? (
                <span className="inline-flex h-6 max-w-48 items-center truncate rounded-md bg-[var(--color-panel-2)] px-2 py-1 text-xs leading-none text-[var(--color-ink-soft)]">
                  {currentProject.name}
                </span>
              ) : null}
            </div>
            <div className="flex items-center gap-1">
              <InfoHint label={systemSummary} />
              <Button asChild variant="ghost" size="icon-xs" aria-label="Open projects">
                <Link to="/projects">
                  <RiFolderOpenLine className="size-3.5" />
                </Link>
              </Button>
              <Button asChild variant="ghost" size="icon-xs" aria-label="Open activity">
                <Link to="/activity">
                  <RiPulseLine className="size-3.5" />
                </Link>
              </Button>
              <Button asChild variant="ghost" size="icon-xs" aria-label="Open settings">
                <Link to="/settings">
                  <RiSettings3Line className="size-3.5" />
                </Link>
              </Button>
            </div>
          </div>

          <div className="flex items-center gap-2 border-b border-[var(--color-line)] px-4 py-2 text-xs text-[var(--color-ink-soft)]">
            <RiLoader4Line className="size-3.5" />
            <span>{statusText}</span>
          </div>

          <div className="px-3 py-3 sm:px-4">
            <ChatMessages data={data} className="h-[30rem] rounded-[0.8rem] border-0 bg-transparent p-0 sm:h-[38rem]" />
          </div>

          <div className="border-t border-[var(--color-line)] px-3 py-3 sm:px-4">
            <div className="mb-2 flex items-center justify-between gap-3">
              <div className="text-[11px] uppercase tracking-[0.18em] text-[var(--color-ink-soft)]">Now</div>
              <TooltipProvider delayDuration={120}>
                <div className="flex items-center gap-1">
                  {primaryPrompt ? (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button type="button" variant="outline" size="icon-sm" aria-label={primaryPrompt.label} onClick={() => void data?.send?.(primaryPrompt.prompt)}>
                          <RiArrowRightUpLine className="size-4" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent sideOffset={6} className="rounded-lg bg-[var(--color-ink)] px-3 py-1.5 text-xs text-[var(--color-panel)]">
                        {primaryPrompt.label}
                      </TooltipContent>
                    </Tooltip>
                  ) : null}
                  {secondaryPrompts.map((prompt) => (
                    <Tooltip key={prompt.label}>
                      <TooltipTrigger asChild>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon-sm"
                          aria-label={prompt.label}
                          onClick={() => void data?.send?.(prompt.prompt)}
                        >
                          {prompt.label.includes('active') ? <RiPulseLine className="size-4" /> : <RiInformationLine className="size-4" />}
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent sideOffset={6} className="rounded-lg bg-[var(--color-ink)] px-3 py-1.5 text-xs text-[var(--color-panel)]">
                        {prompt.label}
                      </TooltipContent>
                    </Tooltip>
                  ))}
                </div>
              </TooltipProvider>
            </div>
            <ChatComposer data={data} embedded />
          </div>
        </CardContent>
      </Card>
    </section>
  );
}
