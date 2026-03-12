import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import { DashboardSection } from './DashboardSection';

const refreshInstances = vi.fn(async () => {});
const refreshExecutions = vi.fn(async () => {});

vi.mock('./components/QuickLaunchCard', () => ({
  QuickLaunchCard: () => <div data-testid="dashboard-quick-launch-card" />,
}));

vi.mock('./components/InstalledAgentsCard', () => ({
  InstalledAgentsCard: () => <div data-testid="dashboard-installed-agents-card" />,
}));

vi.mock('./components/RecentExecutionsCard', () => ({
  RecentExecutionsCard: () => <div data-testid="dashboard-recent-executions-card" />,
}));

vi.mock('./components/AddAgentModal', () => ({
  AddAgentModal: () => <div data-testid="dashboard-add-agent-modal" />,
}));

describe('DashboardSection', () => {
  afterEach(() => {
    cleanup();
    refreshInstances.mockClear();
    refreshExecutions.mockClear();
  });

  test('renders split dashboard sections', () => {
    render(<DashboardSection data={{ refreshInstances, refreshExecutions } as any} />);

    expect(screen.getByTestId('dashboard-quick-launch-card')).toBeInTheDocument();
    expect(screen.getByTestId('dashboard-installed-agents-card')).toBeInTheDocument();
    expect(screen.getByTestId('dashboard-recent-executions-card')).toBeInTheDocument();
    expect(screen.getByTestId('dashboard-add-agent-modal')).toBeInTheDocument();
  });
});
