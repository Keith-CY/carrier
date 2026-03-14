import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import { ExecutionsSection } from './ExecutionsSection';

const refreshExecutions = vi.fn(async () => {});

vi.mock('./components/ExecutionsToolbarCard', () => ({
  ExecutionsToolbarCard: () => <div data-testid="executions-toolbar-card" />,
}));

vi.mock('./components/ExecutionsListCard', () => ({
  ExecutionsListCard: () => <div data-testid="executions-list-card" />,
}));

vi.mock('./components/ExecutionDetailCard', () => ({
  ExecutionDetailCard: () => <div data-testid="execution-detail-card" />,
}));

describe('ExecutionsSection', () => {
  afterEach(() => {
    cleanup();
    refreshExecutions.mockClear();
  });

  test('renders split execution components and refreshes through data hook output', () => {
    render(
      <ExecutionsSection
        data={{
          refreshExecutions,
        } as any}
      />,
    );

    expect(screen.getByRole('heading', { name: 'Executions' })).toBeInTheDocument();
    expect(screen.getByTestId('executions-toolbar-card')).toBeInTheDocument();
    expect(screen.getByTestId('executions-list-card')).toBeInTheDocument();
    expect(screen.getByTestId('execution-detail-card')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(refreshExecutions).toHaveBeenCalledTimes(1);
  });
});
