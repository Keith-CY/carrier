import { useMemo, useState } from 'react';
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import {
  RiAccountCircleLine,
  RiArrowRightUpLine,
  RiCheckboxCircleLine,
  RiChat4Line,
  RiCommandLine,
  RiDatabase2Line,
  RiMenuLine,
  RiPulseLine,
  RiServerLine,
  RiShieldCheckLine,
  RiShutDownLine,
  RiSparklingLine,
  RiTerminalBoxLine,
} from 'react-icons/ri';
import { CarrierHeaderLogo } from '@/components/carrier-header-logo';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
} from '@/components/ui/command';
import { Input } from '@/components/ui/input';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Sheet, SheetContent, SheetTrigger } from '@/components/ui/sheet';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { navItems } from './navigation';
import { useSession } from './session';

function healthToneClass(text: string) {
  const normalized = text.toLowerCase();
  if (normalized.includes('online')) return 'bg-[var(--color-panel-2)] text-[var(--color-ink)]';
  if (normalized.includes('offline')) return 'bg-[var(--color-panel)] text-[var(--color-ink-soft)]';
  return 'bg-[var(--color-panel-2)] text-[var(--color-ink-soft)]';
}

function ShellNavigation({ onNavigate, compact = false }: { onNavigate?: () => void; compact?: boolean }) {
  return (
    <TooltipProvider delayDuration={120}>
      <nav className={compact ? 'grid gap-2' : 'space-y-2'}>
      {navItems.map((item) => {
        const Icon = item.icon;
        const link = (
          <NavLink
            key={item.to}
            to={item.to}
            onClick={onNavigate}
            aria-label={item.label}
            className={({ isActive }) =>
              compact
                ? [
                    'flex size-10 items-center justify-center rounded-xl border transition-colors',
                    isActive
                      ? 'border-[var(--color-line-strong)] bg-[var(--color-panel)] text-[var(--color-ink)]'
                      : 'border-transparent text-[var(--color-ink-soft)] hover:border-[var(--color-line)] hover:bg-[var(--color-panel)] hover:text-[var(--color-ink)]',
                  ].join(' ')
                : [
                    'group flex items-center gap-3 rounded-xl border px-3 py-2.5 transition-colors',
                    isActive
                      ? 'border-[var(--color-line-strong)] bg-[var(--color-panel)] text-[var(--color-ink)]'
                      : 'border-transparent bg-transparent text-[var(--color-ink-soft)] hover:border-[var(--color-line)] hover:bg-[var(--color-panel)] hover:text-[var(--color-ink)]',
                  ].join(' ')
            }
          >
            <Icon className="size-4.5" />
            {compact ? <span className="sr-only">{item.label}</span> : <span className="text-sm font-medium">{item.label}</span>}
          </NavLink>
        );
        if (!compact) return link;
        return (
          <Tooltip key={item.to}>
            <TooltipTrigger asChild>{link}</TooltipTrigger>
            <TooltipContent side="right" sideOffset={8} className="rounded-lg bg-[var(--color-ink)] px-3 py-1.5 text-xs text-[var(--color-panel)]">
              {item.label}
            </TooltipContent>
          </Tooltip>
        );
      })}
      </nav>
    </TooltipProvider>
  );
}

const legacyShellItems = [
  { match: /^\/(?:welcome|setup|provider|config|install|complete|add\/)/, label: 'Onboarding', icon: RiCheckboxCircleLine },
  { match: /^\/dashboard(?:\/|$)/, label: 'Dashboard', icon: RiSparklingLine },
  { match: /^\/hosts(?:\/|$)/, label: 'Hosts', icon: RiServerLine },
  { match: /^\/providers(?:\/|$)/, label: 'Providers', icon: RiSparklingLine },
  { match: /^\/remote-chat(?:\/|$)/, label: 'Remote Chat', icon: RiChat4Line },
  { match: /^\/executions(?:\/|$)/, label: 'Executions', icon: RiPulseLine },
  { match: /^\/memory(?:\/|$)/, label: 'Memory', icon: RiDatabase2Line },
  { match: /^\/logs(?:\/|$)/, label: 'Logs', icon: RiTerminalBoxLine },
  { match: /^\/workers(?:\/|$)/, label: 'Workers', icon: RiServerLine },
  { match: /^\/policies(?:\/|$)/, label: 'Policies', icon: RiShieldCheckLine },
  { match: /^\/remote-observability(?:\/|$)/, label: 'Observability', icon: RiPulseLine },
  { match: /^\/work(?:\/|$)/, label: 'Projects', icon: navItems[1].icon },
  { match: /^\/chat(?:\/|$)/, label: 'Home', icon: navItems[0].icon },
];

export function AppLayout() {
  const navigate = useNavigate();
  const location = useLocation();
  const { authenticated, clearLoginError, dismissToast, health, login, loginError, logout, toasts } = useSession();
  const [tokenDraft, setTokenDraft] = useState('');
  const [commandOpen, setCommandOpen] = useState(false);
  const compactMode = location.pathname === '/quick-entry';
  const loginTargetText = compactMode ? 'Inbox opens first.' : 'Home opens first.';

  const currentNavItem = useMemo(
    () =>
      navItems.find((item) => location.pathname === item.to || location.pathname.startsWith(`${item.to}/`)) ||
      legacyShellItems.find((item) => item.match.test(location.pathname)) ||
      navItems[0],
    [location.pathname],
  );

  const connect = async () => {
    const ok = await login(tokenDraft);
    if (ok) setTokenDraft('');
  };

  return (
    <>
      <div className="min-h-screen">
        {!authenticated ? (
          <div id="login-overlay" className="fixed inset-0 z-50 grid place-items-center bg-[rgba(243,239,231,0.82)] p-6 backdrop-blur-sm">
            <Card className="w-full max-w-lg rounded-[1.1rem] border-[color:rgba(20,24,31,0.08)] bg-[rgba(251,250,246,0.96)] shadow-[0_18px_48px_rgba(18,23,31,0.08)]">
              <CardContent className="space-y-5 p-7">
                <div className="flex items-center gap-2">
                  <span className="flex size-9 items-center justify-center rounded-[0.85rem] border border-[color:rgba(20,24,31,0.08)] bg-[var(--color-panel)] text-[var(--color-primary)]">
                    <CarrierHeaderLogo className="size-5" />
                  </span>
                  <div className="text-base font-medium text-[var(--color-ink)]">Carrier</div>
                </div>
                <div className="space-y-2">
                  <h1 className="text-2xl font-semibold leading-tight">Connect</h1>
                  <p className="text-sm leading-6 text-[var(--color-ink-soft)]">
                    Use the gateway token for this environment.
                  </p>
                </div>
                <div className="space-y-3">
                  <Input
                    id="login-token"
                    type="password"
                    placeholder="Gateway token"
                    autoComplete="off"
                    value={tokenDraft}
                    onChange={(event) => {
                      clearLoginError();
                      setTokenDraft(event.target.value);
                    }}
                    onKeyDown={(event) => {
                      if (event.key !== 'Enter') return;
                      event.preventDefault();
                      void connect();
                    }}
                  />
                  <div id="login-msg" className={loginError ? 'text-sm text-[var(--color-danger)]' : 'text-sm text-[var(--color-ink-soft)]'}>
                    {loginError || 'Use the same token you would use against this gateway.'}
                  </div>
                </div>
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="text-xs text-[var(--color-ink-soft)]">{loginTargetText}</div>
                  <Button id="login-btn" onClick={() => void connect()} size="sm">
                    Connect
                    <RiArrowRightUpLine className="size-4" />
                  </Button>
                </div>
              </CardContent>
            </Card>
          </div>
        ) : null}

        <div className={authenticated ? '' : 'pointer-events-none select-none opacity-25 blur-sm'}>
          <div className={compactMode
            ? 'mx-auto min-h-screen max-w-[40rem] px-3 py-3 sm:px-5 sm:py-5'
            : 'mx-auto flex min-h-screen max-w-[1480px] gap-4 px-3 py-3 lg:px-5 lg:py-5'}
          >
            {compactMode ? (
              <main id="main">
                <Outlet />
              </main>
            ) : (
              <>
            <aside className="hidden w-[4.5rem] shrink-0 lg:block">
              <div className="sticky top-5 flex min-h-[calc(100vh-2.5rem)] flex-col items-center justify-between rounded-[1rem] border border-[var(--color-line)] bg-[rgba(251,250,246,0.9)] p-2 shadow-[0_10px_30px_rgba(45,41,36,0.04)]">
                <div className="grid gap-3">
                  <div className="flex size-10 items-center justify-center rounded-xl bg-[var(--color-panel-2)] text-[var(--color-primary)]">
                    <CarrierHeaderLogo className="size-5" />
                  </div>
                  <ShellNavigation compact />
                </div>

                <div className="grid gap-2">
                  <TooltipProvider delayDuration={120}>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <div className={`flex size-10 items-center justify-center rounded-xl ${healthToneClass(health.text)}`}>
                          <span className="size-2.5 rounded-full bg-current" />
                        </div>
                      </TooltipTrigger>
                      <TooltipContent side="right" sideOffset={8} className="rounded-lg bg-[var(--color-ink)] px-3 py-1.5 text-xs text-[var(--color-panel)]">
                        {health.text}
                      </TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                </div>
              </div>
            </aside>

            <div className="min-w-0 flex-1">
              <div id="app-header" className="mb-3 flex items-center justify-between gap-3 rounded-[1rem] border border-[var(--color-line)] bg-[rgba(251,250,246,0.92)] px-3 py-2 shadow-[0_10px_30px_rgba(45,41,36,0.04)] lg:px-4">
                <div className="flex items-center gap-3">
                  <Sheet>
                    <SheetTrigger asChild>
                      <Button variant="outline" size="icon-sm" className="lg:hidden" aria-label="Open navigation">
                        <RiMenuLine className="size-4" />
                      </Button>
                    </SheetTrigger>
                    <SheetContent side="left" className="w-[18rem] bg-[var(--color-panel)] p-4">
                      <div className="mb-4 flex items-center gap-2 text-sm font-medium text-[var(--color-ink)]">
                        <span className="flex size-7 items-center justify-center rounded-lg bg-[var(--color-panel-2)] text-[var(--color-primary)]">
                          <CarrierHeaderLogo className="size-4" />
                        </span>
                        Carrier
                      </div>
                      <ShellNavigation onNavigate={() => {}} compact={false} />
                    </SheetContent>
                  </Sheet>

                  <div className="hidden items-center gap-2 rounded-lg border border-[var(--color-line)] bg-[var(--color-panel)] px-2 py-1 text-[var(--color-primary)] sm:flex">
                    <CarrierHeaderLogo className="size-4" />
                    <span className="text-[0.7rem] font-semibold uppercase tracking-[0.12em] text-[var(--color-ink)]">Carrier</span>
                  </div>
                  <span className="hidden h-5 w-px bg-[var(--color-line)] sm:block" />
                  <span className="flex size-8 items-center justify-center rounded-lg bg-[var(--color-panel-2)] text-[var(--color-primary)]">
                    <currentNavItem.icon className="size-4" />
                  </span>
                  <div className="text-sm font-medium text-[var(--color-ink)]">{currentNavItem.label}</div>
                </div>

                <div className="flex items-center gap-2">
                  <Button variant="ghost" size="icon-sm" className="hidden sm:inline-flex" onClick={() => setCommandOpen(true)} aria-label="Open command menu">
                    <RiCommandLine className="size-4" />
                  </Button>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon-sm" aria-label="Open account menu">
                        <RiAccountCircleLine className="size-4.5" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-56">
                      <DropdownMenuItem onClick={() => navigate('/settings')}>Open settings</DropdownMenuItem>
                      <DropdownMenuItem onClick={() => logout()}>
                        <RiShutDownLine className="mr-2 size-4" />
                        Disconnect
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </div>

              <main id="main">
                <Outlet />
              </main>
            </div>
              </>
            )}
          </div>
        </div>
      </div>

      {!compactMode ? <CommandDialog open={commandOpen} onOpenChange={setCommandOpen}>
        <CommandInput placeholder="Jump to a page or action…" />
        <CommandList>
          <CommandEmpty>No matching destinations.</CommandEmpty>
          <CommandGroup heading="Navigate">
            {navItems.map((item) => {
              const Icon = item.icon;
              return (
                <CommandItem
                  key={item.to}
                  onSelect={() => {
                    navigate(item.to);
                    setCommandOpen(false);
                  }}
                >
                  <Icon className="size-4" />
                  <span>{item.label}</span>
                  <CommandShortcut>{item.label === 'Home' ? 'H' : ''}</CommandShortcut>
                </CommandItem>
              );
            })}
          </CommandGroup>
          <CommandSeparator />
          <CommandGroup heading="Actions">
            <CommandItem
              onSelect={() => {
                navigate('/home');
                setCommandOpen(false);
              }}
            >
              <RiSparklingLine className="size-4" />
              <span>Start from Home</span>
            </CommandItem>
            <CommandItem
              onSelect={() => {
                logout();
                setCommandOpen(false);
              }}
            >
              <RiShutDownLine className="size-4" />
              <span>Disconnect gateway</span>
            </CommandItem>
          </CommandGroup>
        </CommandList>
      </CommandDialog> : null}

      {toasts.length ? (
        <div className="fixed bottom-5 right-5 z-50 grid max-w-md gap-2">
          {toasts.map((toast) => (
            <Card key={toast.id} className="rounded-[0.9rem] border-[var(--color-line)] bg-white/96 shadow-[0_12px_30px_rgba(45,41,36,0.08)]">
              <CardContent className="flex items-start justify-between gap-3 p-4">
                <div className="text-sm leading-7 text-[var(--color-ink)]">{toast.text}</div>
                <Button variant="ghost" size="sm" onClick={() => dismissToast(toast.id)}>Dismiss</Button>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : null}
    </>
  );
}
