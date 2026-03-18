import { useQuery } from '@tanstack/react-query';
import { RiArrowRightUpLine, RiSettings3Line } from 'react-icons/ri';
import { Link } from 'react-router-dom';
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { InfoHint } from '@/components/ui/info-hint';
import { usePWAInstall } from '@/app/pwa';
import { apiGet, type ChannelStatusPayload, type ProviderAuthStatusPayload } from '@/lib/api';
import { useSettingsData } from './useSettingsData';

export function SettingsHubPage() {
  const data = useSettingsData();
  const install = usePWAInstall();
  const providerStatusQuery = useQuery({
    queryKey: ['settings-hub', 'providers'],
    queryFn: () => apiGet<ProviderAuthStatusPayload>('/api/v1/auth/providers'),
    retry: false,
  });
  const channelStatusQuery = useQuery({
    queryKey: ['settings-hub', 'channels'],
    queryFn: () => apiGet<ChannelStatusPayload>('/api/v1/channels'),
    retry: false,
  });

  const providers = Array.isArray(providerStatusQuery.data?.providers) ? providerStatusQuery.data.providers : [];
  const channels = Array.isArray(channelStatusQuery.data?.channels) ? channelStatusQuery.data.channels : [];

  return (
    <section id="view-settings" className="space-y-6">
      <Card className="rounded-[1rem] border-[var(--color-line)] bg-[rgba(251,250,246,0.92)] shadow-[0_10px_30px_rgba(45,41,36,0.04)]">
        <CardContent className="flex min-h-8 items-center gap-2 p-5 sm:p-6">
          <span className="flex size-8 items-center justify-center rounded-lg bg-[var(--color-panel-2)] text-[var(--color-primary)]">
            <RiSettings3Line className="size-4" />
          </span>
          <h1 className="text-xl leading-none font-medium text-[var(--color-ink)]">Settings</h1>
          <InfoHint label="Keep setup and integration details here. Home should stay focused on conversation and task flow." />
        </CardContent>
      </Card>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.05fr)_minmax(0,0.95fr)]">
        <Card className="rounded-[1rem] border-[var(--color-line)] bg-white/88">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <RiSettings3Line className="size-5 text-[var(--color-primary)]" />
              Setup health
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4 text-sm leading-7 text-[var(--color-ink-soft)]">
            <p>{data.summary}</p>
            <div className="flex flex-wrap gap-2">
              <Button asChild size="sm">
                <Link to="/home">
                  Back to Home
                  <RiArrowRightUpLine className="size-4" />
                </Link>
              </Button>
              {install.canInstall ? <Button variant="outline" size="sm" onClick={() => void install.promptInstall()}>Install Inbox</Button> : null}
              <Button asChild variant="outline" size="sm">
                <Link to="/quick-entry">Open Inbox</Link>
              </Button>
              <Button variant="outline" size="sm" onClick={() => data.logout()}>Disconnect</Button>
            </div>
          </CardContent>
        </Card>

        <div className="grid gap-4">
          <Card className="rounded-[1rem] border-[var(--color-line)] bg-white/88">
            <CardHeader>
              <CardTitle>Providers</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {providers.map((provider) => (
                <div key={provider.id} className="flex items-center justify-between gap-3 rounded-[0.8rem] border border-[var(--color-line)] bg-[var(--color-panel)] px-4 py-3">
                  <div>
                    <div className="font-medium text-[var(--color-ink)]">{provider.name || provider.id}</div>
                    <div className="text-sm text-[var(--color-ink-soft)]">{provider.authMode || 'unknown auth mode'}</div>
                  </div>
                  <span className="shrink-0 self-center text-right text-xs leading-none text-[var(--color-ink-soft)]">{provider.configured ? 'Connected' : 'Pending'}</span>
                </div>
              ))}
            </CardContent>
          </Card>

          <Card className="rounded-[1rem] border-[var(--color-line)] bg-white/88">
            <CardHeader>
              <CardTitle>Channels</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {channels.map((channel) => (
                <div key={channel.id} className="flex items-center justify-between gap-3 rounded-[0.8rem] border border-[var(--color-line)] bg-[var(--color-panel)] px-4 py-3">
                  <div>
                    <div className="font-medium text-[var(--color-ink)]">{channel.displayName || channel.id}</div>
                    <div className="text-sm text-[var(--color-ink-soft)]">{channel.supportsWebUI ? 'WebUI supported' : 'CLI only'}</div>
                  </div>
                  <span className="shrink-0 self-center text-right text-xs leading-none text-[var(--color-ink-soft)]">{channel.configured ? 'Configured' : 'Pending'}</span>
                </div>
              ))}
            </CardContent>
          </Card>
        </div>
      </div>

      <Card className="rounded-[1rem] border-[var(--color-line)] bg-white/88">
        <CardHeader>
          <CardTitle>Advanced</CardTitle>
        </CardHeader>
        <CardContent>
          <Accordion type="single" collapsible className="w-full">
            <AccordionItem value="defaults">
              <AccordionTrigger>What lives behind the simple surface?</AccordionTrigger>
              <AccordionContent className="space-y-2 text-sm text-[var(--color-ink-soft)]">
                <div>Provider bindings and runtime defaults stay behind this layer.</div>
                <div>Policy behavior and rollout traces should only appear when you are actively debugging or approving work.</div>
                <div>That separation keeps Home and Projects approachable for a first-time user.</div>
              </AccordionContent>
            </AccordionItem>
          </Accordion>
        </CardContent>
      </Card>
    </section>
  );
}
