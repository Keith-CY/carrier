import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import { OnboardingSection } from './OnboardingSection';

vi.mock('./components/WelcomeStep', () => ({ WelcomeStep: () => <div data-testid="welcome-step" /> }));
vi.mock('./components/SetupStep', () => ({ SetupStep: () => <div data-testid="setup-step" /> }));
vi.mock('./components/AgentsStep', () => ({ AgentsStep: () => <div data-testid="agents-step" /> }));
vi.mock('./components/ProviderStep', () => ({ ProviderStep: () => <div data-testid="provider-step" /> }));
vi.mock('./components/ConfigStep', () => ({ ConfigStep: () => <div data-testid="config-step" /> }));
vi.mock('./components/InstallStep', () => ({ InstallStep: () => <div data-testid="install-step" /> }));
vi.mock('./components/CompleteStep', () => ({ CompleteStep: () => <div data-testid="complete-step" /> }));

describe('OnboardingSection', () => {
  afterEach(() => cleanup());

  test('renders requested step component', () => {
    render(<OnboardingSection data={{ step: 'provider' } as any} />);
    expect(screen.getByTestId('provider-step')).toBeInTheDocument();
  });
});
