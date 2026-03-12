import { AgentsStep } from './components/AgentsStep';
import { CompleteStep } from './components/CompleteStep';
import { ConfigStep } from './components/ConfigStep';
import { InstallStep } from './components/InstallStep';
import { ProviderStep } from './components/ProviderStep';
import { SetupStep } from './components/SetupStep';
import { WelcomeStep } from './components/WelcomeStep';
import type { OnboardingData } from './useOnboardingData';

export function OnboardingSection({ data }: { data: OnboardingData }) {
  if (data.step === 'welcome') return <WelcomeStep data={data} />;
  if (data.step === 'setup') return <SetupStep data={data} />;
  if (data.step === 'agents') return <AgentsStep data={data} />;
  if (data.step === 'provider') return <ProviderStep data={data} />;
  if (data.step === 'config') return <ConfigStep data={data} />;
  if (data.step === 'install') return <InstallStep data={data} />;
  return <CompleteStep data={data} />;
}
