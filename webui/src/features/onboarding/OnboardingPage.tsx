import { OnboardingSection } from './OnboardingSection';
import type { Step } from './model';
import { useOnboardingData } from './useOnboardingData';

export function OnboardingPage({ step, addTargetAgent }: { step: Step; addTargetAgent?: string }) {
  const data = useOnboardingData({ step, addTargetAgent });
  return <OnboardingSection data={data} />;
}
