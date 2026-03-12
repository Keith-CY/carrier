import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import { LogsSection } from './LogsSection';

vi.mock('./components/LogsToolbarCard', () => ({
  LogsToolbarCard: () => <div data-testid="logs-toolbar-card" />,
}));

vi.mock('./components/LogsFilters', () => ({
  LogsFilters: () => <div data-testid="logs-filters" />,
}));

vi.mock('./components/LogsTable', () => ({
  LogsTable: () => <div data-testid="logs-table" />,
}));

describe('LogsSection', () => {
  afterEach(() => {
    cleanup();
  });

  test('renders split log components', () => {
    render(<LogsSection data={{} as any} />);

    expect(screen.getByRole('heading', { name: 'Logs' })).toBeInTheDocument();
    expect(screen.getByTestId('logs-toolbar-card')).toBeInTheDocument();
    expect(screen.getByTestId('logs-filters')).toBeInTheDocument();
    expect(screen.getByTestId('logs-table')).toBeInTheDocument();
  });
});
