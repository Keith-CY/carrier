import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import { WorkersSection } from './WorkersSection';

const refreshWorkers = vi.fn(async () => {});
const reclaimIdle = vi.fn();
const reclaimStale = vi.fn();

vi.mock('./components/WorkersToolbarCard', () => ({
  WorkersToolbarCard: () => <div data-testid="workers-toolbar-card" />,
}));

vi.mock('./components/WorkersList', () => ({
  WorkersList: () => <div data-testid="workers-list-card" />,
}));

describe('WorkersSection', () => {
  afterEach(() => {
    cleanup();
    refreshWorkers.mockClear();
    reclaimIdle.mockClear();
    reclaimStale.mockClear();
  });

  test('renders split worker components and top-level actions', () => {
    render(
      <WorkersSection
        data={{
          refreshWorkers,
          reclaimIdle,
          reclaimStale,
        } as any}
      />,
    );

    expect(screen.getByRole('heading', { name: 'Workers' })).toBeInTheDocument();
    expect(screen.getByTestId('workers-toolbar-card')).toBeInTheDocument();
    expect(screen.getByTestId('workers-list-card')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Reclaim Stale' }));
    fireEvent.click(screen.getByRole('button', { name: 'Reclaim Idle' }));
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));

    expect(reclaimStale).toHaveBeenCalledTimes(1);
    expect(reclaimIdle).toHaveBeenCalledTimes(1);
    expect(refreshWorkers).toHaveBeenCalledTimes(1);
  });
});
