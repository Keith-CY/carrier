import { RiInformationLine } from 'react-icons/ri';
import { cn } from '@/lib/utils';
import { Button } from './button';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from './tooltip';

export function InfoHint({ label, className }: { label: string; className?: string }) {
  return (
    <TooltipProvider delayDuration={120}>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            aria-label={label}
            className={cn('shrink-0 self-center text-[var(--color-ink-soft)] hover:bg-[var(--color-panel-2)] hover:text-[var(--color-ink)]', className)}
          >
            <RiInformationLine className="size-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent sideOffset={6} className="max-w-64 rounded-lg bg-[var(--color-ink)] px-3 py-2 text-[11px] leading-5 text-[var(--color-panel)]">
          {label}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
