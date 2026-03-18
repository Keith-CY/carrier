import { RiInformationLine, RiRobot2Line, RiSparklingLine, RiUser3Line } from 'react-icons/ri';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet';
import { cn } from '@/lib/utils';
import type { ChatData } from '../useChatData';

function bubbleTone(role: string) {
  if (role === 'user') return 'bg-[var(--color-chat-user)] border-[var(--color-line)]';
  if (role === 'assistant') return 'bg-[var(--color-chat-assistant)] border-[var(--color-line)]';
  return 'bg-[var(--color-panel-2)] border-[color:rgba(20,24,31,0.08)]';
}

function roleLabel(role: string) {
  if (role === 'user') return 'You';
  if (role === 'assistant') return 'Base agent';
  return 'System';
}

function RoleIcon({ role }: { role: string }) {
  if (role === 'user') return <RiUser3Line className="size-4" />;
  if (role === 'assistant') return <RiSparklingLine className="size-4" />;
  return <RiRobot2Line className="size-4" />;
}

export function ChatMessages({ data, className }: { data: ChatData; className?: string }) {
  return (
    <ScrollArea className={cn('h-[26rem] rounded-[1rem] border border-[var(--color-line)] bg-[rgba(251,250,246,0.72)] p-4 sm:h-[34rem]', className)}>
      <div id="chat-messages" className="space-y-4">
        {data.messages.map((message) => (
          <div
            key={message.id}
            className={`rounded-[0.85rem] border px-4 py-4 shadow-[0_4px_12px_rgba(45,41,36,0.03)] ${bubbleTone(message.role)}`}
          >
            <div className="mb-3 flex items-start justify-between gap-3">
              <div className="flex items-center gap-2 text-sm font-medium text-[var(--color-ink)]">
                <span className="flex size-8 items-center justify-center rounded-full bg-white/70">
                  <RoleIcon role={message.role} />
                </span>
                <span>{roleLabel(message.role)}</span>
              </div>
              <div className="flex items-center gap-2">
                {message.sessionId ? <Badge variant="secondary">session {message.sessionId}</Badge> : null}
                {(message.sessionId || message.requestId) ? (
                  <Sheet>
                    <SheetTrigger asChild>
                      <Button variant="ghost" size="icon-xs" aria-label={`${roleLabel(message.role)} details`}>
                        <RiInformationLine className="size-3.5" />
                      </Button>
                    </SheetTrigger>
                    <SheetContent className="w-full sm:max-w-md">
                      <SheetHeader>
                        <SheetTitle>{roleLabel(message.role)} details</SheetTitle>
                      </SheetHeader>
                      <div className="mt-6 space-y-3 text-sm text-[var(--color-ink-soft)]">
                        <div>
                          <div className="font-medium text-[var(--color-ink)]">Session</div>
                          <div>{message.sessionId || 'not assigned yet'}</div>
                        </div>
                        <div>
                          <div className="font-medium text-[var(--color-ink)]">Request</div>
                          <div>{message.requestId || 'not exposed'}</div>
                        </div>
                        <div>
                          <div className="font-medium text-[var(--color-ink)]">Created</div>
                          <div>{new Date(message.createdAt).toLocaleString()}</div>
                        </div>
                      </div>
                    </SheetContent>
                  </Sheet>
                ) : null}
              </div>
            </div>
            <div className="whitespace-pre-wrap text-[15px] leading-7 text-[var(--color-ink)]">{message.text}</div>
          </div>
        ))}
      </div>
    </ScrollArea>
  );
}
