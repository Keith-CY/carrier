import { Link } from 'react-router-dom';
import { RiArrowRightUpLine, RiCheckboxCircleLine, RiRobot2Line, RiSettings3Line, RiSparklingLine } from 'react-icons/ri';
import { InfoHint } from '@/components/ui/info-hint';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';

const steps = [
  {
    title: 'Connect Carrier',
    description: 'Use the gateway token for this environment and unlock the WebUI.',
    icon: RiSettings3Line,
  },
  {
    title: 'Choose your agent style',
    description: 'Start with the base agent first. Reach for deeper surfaces only when the work becomes more complex.',
    icon: RiRobot2Line,
  },
  {
    title: 'Connect an AI provider',
    description: 'Set up the provider you trust. The simple surface should still hide the wiring after this step.',
    icon: RiSparklingLine,
  },
  {
    title: 'Run the first task',
    description: 'Land in Home and ask for the first task, status check, or next best step.',
    icon: RiCheckboxCircleLine,
  },
];

export function OnboardingHubPage() {
  return (
    <section id="view-onboarding" className="space-y-6">
      <Card className="rounded-[1.1rem] border-[color:rgba(20,24,31,0.08)] bg-[rgba(251,250,246,0.92)] shadow-[0_12px_32px_rgba(18,23,31,0.04)]">
        <CardContent className="space-y-6 p-6 sm:p-7">
          <div className="flex min-h-8 items-center gap-2">
            <span className="flex size-8 items-center justify-center rounded-lg border border-[color:rgba(20,24,31,0.08)] bg-[var(--color-panel)] text-[var(--color-primary)]">
              <RiCheckboxCircleLine className="size-4" />
            </span>
            <h1 className="text-xl leading-none font-semibold tracking-[0.01em] text-[var(--color-ink)]">Start</h1>
            <InfoHint label="Finish the minimum setup, then move straight into Home." />
          </div>

          <div className="grid gap-2 sm:grid-cols-4">
            {steps.map((step, index) => (
              <div key={step.title} className="rounded-[0.9rem] border border-[color:rgba(20,24,31,0.08)] bg-white/86 px-3 py-2 text-sm text-[var(--color-ink-soft)]">
                <span className="mr-2 font-medium text-[var(--color-ink)]">{index + 1}.</span>
                {step.title}
              </div>
            ))}
          </div>

          <div className="grid gap-3 xl:grid-cols-2">
            {steps.map((step, index) => {
              const Icon = step.icon;
              return (
                <div key={step.title} className="rounded-[0.95rem] border border-[color:rgba(20,24,31,0.08)] bg-white/90 p-4">
                  <div className="mb-3 flex min-h-8 items-center gap-3">
                    <span className="flex size-8 items-center justify-center rounded-lg border border-[color:rgba(20,24,31,0.08)] bg-[var(--color-panel)] text-[var(--color-primary)]">
                      <Icon className="size-4" />
                    </span>
                    <div className="text-xs uppercase tracking-[0.18em] text-[var(--color-ink-soft)]">Step {index + 1}</div>
                  </div>
                  <div className="text-lg font-medium text-[var(--color-ink)]">{step.title}</div>
                  <div className="mt-2 text-sm leading-6 text-[var(--color-ink-soft)]">{step.description}</div>
                </div>
              );
            })}
          </div>

          <div className="flex flex-wrap gap-2">
            <Button asChild size="sm">
              <Link to="/home">
                Open Home
                <RiArrowRightUpLine className="size-4" />
              </Link>
            </Button>
            <Button asChild variant="outline" size="sm">
              <Link to="/settings">Settings</Link>
            </Button>
          </div>
        </CardContent>
      </Card>
    </section>
  );
}
