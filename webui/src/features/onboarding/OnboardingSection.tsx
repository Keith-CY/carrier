import { PageShell } from '../../app/page-shell';
import type { Step } from './model';
import { AgentsStep } from './components/AgentsStep';
import { CompleteStep } from './components/CompleteStep';
import { ConfigStep } from './components/ConfigStep';
import { InstallStep } from './components/InstallStep';
import { ProviderStep } from './components/ProviderStep';
import { SetupStep } from './components/SetupStep';
import { WelcomeStep } from './components/WelcomeStep';
import type { OnboardingData } from './useOnboardingData';

const STEP_COPY: Array<{ id: Step; title: string; meta: string; description: string }> = [
  {
    id: 'welcome',
    title: 'Connect Gateway',
    meta: 'Verify local access and daemon health',
    description: 'Begin with the gateway handshake so the rest of the setup flow starts from a known-good control plane.',
  },
  {
    id: 'setup',
    title: 'Choose Channel',
    meta: 'Pair the operator channel and webhook inputs',
    description: 'Capture the operator-facing channel that Carrier uses for interaction, routing, and channel pairing.',
  },
  {
    id: 'agents',
    title: 'Select Agent',
    meta: 'Pick the runtime surface to install next',
    description: 'Choose the agent profile you want to activate so the rest of the flow can tailor provider and install behavior.',
  },
  {
    id: 'provider',
    title: 'Attach Provider',
    meta: 'Reuse or inject credentials deliberately',
    description: 'Choose the LLM provider posture for the selected agent, with reusable credentials when Carrier can supply them.',
  },
  {
    id: 'config',
    title: 'Review Config',
    meta: 'Inspect environment variables and runtime defaults',
    description: 'Confirm the environment Carrier will hand to the agent before you commit the install step.',
  },
  {
    id: 'install',
    title: 'Install Agent',
    meta: 'Run the final install and sync sequence',
    description: 'Launch the install with the configuration you assembled and verify the resulting runtime payload.',
  },
  {
    id: 'complete',
    title: 'Go Live',
    meta: 'Verify completion and hand off to operations',
    description: 'Finish the onboarding flow with a clean summary and a direct path back into daily operations.',
  },
];

function renderStep(data: OnboardingData) {
  if (data.step === 'welcome') return <WelcomeStep data={data} />;
  if (data.step === 'setup') return <SetupStep data={data} />;
  if (data.step === 'agents') return <AgentsStep data={data} />;
  if (data.step === 'provider') return <ProviderStep data={data} />;
  if (data.step === 'config') return <ConfigStep data={data} />;
  if (data.step === 'install') return <InstallStep data={data} />;
  return <CompleteStep data={data} />;
}

export function OnboardingSection({ data }: { data: OnboardingData }) {
  const activeIndex = Math.max(STEP_COPY.findIndex((step) => step.id === data.step), 0);
  const activeStep = STEP_COPY[activeIndex] || STEP_COPY[0];
  const selectedAgentName = data.selectedAgent
    ? data.agentDisplayName(data.selectedAgent)
    : data.resolvedAddTargetAgent
      ? data.agentDisplayName(data.resolvedAddTargetAgent)
      : 'Pending';

  return (
    <section className="onboarding-shell">
      <aside className="card onboarding-shell__rail">
        <div>
          <div className="app-brand__eyebrow">Setup Journey</div>
          <div className="onboarding-shell__rail-title">
            {data.addMode ? `Add ${data.agentDisplayName(data.resolvedAddTargetAgent || data.selectedAgent || '')}` : 'Provision Carrier'}
          </div>
          <p className="text-dim" style={{ marginTop: '10px' }}>
            Commercial-grade onboarding should feel sequenced, not improvised. This rail keeps the journey legible end to end.
          </p>
        </div>
        <div className="onboarding-shell__steps">
          {STEP_COPY.map((step, index) => {
            const isActive = step.id === data.step;
            const isComplete = index < activeIndex;
            return (
              <div key={step.id} className={`onboarding-step${isActive ? ' is-active' : ''}${isComplete ? ' is-complete' : ''}`}>
                <div className="onboarding-step__index">Step {index + 1}</div>
                <div className="onboarding-step__title">{step.title}</div>
                <div className="onboarding-step__meta">{step.meta}</div>
              </div>
            );
          })}
        </div>
      </aside>

      <div className="onboarding-shell__content">
        <PageShell
          id="view-onboarding-shell"
          className="page-shell--onboarding"
          eyebrow="Configure"
          title={activeStep.title}
          description={activeStep.description}
          stats={[
            { label: 'Journey', value: data.addMode ? 'Add Agent' : 'Initial Setup' },
            { label: 'Stage', value: `${activeIndex + 1} / ${STEP_COPY.length}` },
            { label: 'Agent', value: selectedAgentName },
          ]}
        >
          <div className="onboarding-stage">
            {renderStep(data)}
          </div>
        </PageShell>
      </div>
    </section>
  );
}
