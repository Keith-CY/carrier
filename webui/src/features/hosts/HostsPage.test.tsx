import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import { HostsPage } from './HostsPage';

const refresh = vi.fn(async () => {});

vi.mock('./useHostsData', () => ({
  useHostsData: () => ({
    featuresLoading: false,
    refresh,
  }),
}));

vi.mock('./HostsSections', () => ({
  HostEditorCard: () => <div data-testid="host-editor-card" />,
  HostsList: () => <div data-testid="hosts-list" />,
  HostManagePanel: () => <div data-testid="host-manage-panel" />,
}));

describe('HostsPage', () => {
  afterEach(() => {
    cleanup();
    refresh.mockClear();
  });

  test('renders split host sections and refreshes through hook', () => {
    render(<HostsPage />);

    expect(screen.getByRole('heading', { name: 'Hosts' })).toBeInTheDocument();
    expect(screen.getByTestId('host-editor-card')).toBeInTheDocument();
    expect(screen.getByTestId('hosts-list')).toBeInTheDocument();
    expect(screen.getByTestId('host-manage-panel')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(refresh).toHaveBeenCalledTimes(1);
  });
});
