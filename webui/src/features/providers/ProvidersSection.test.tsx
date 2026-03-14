import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import { ProvidersSection } from './ProvidersSection';

const refreshAll = vi.fn(async () => {});

vi.mock('./useProvidersData', () => ({
  useProvidersData: () => ({
    refreshAll,
  }),
}));

vi.mock('./components/ProviderProfileEditor', () => ({
  ProviderProfileEditor: () => <div data-testid="provider-profile-editor" />,
}));

vi.mock('./components/ProviderBindingsCard', () => ({
  ProviderBindingsCard: () => <div data-testid="provider-bindings-card" />,
}));

vi.mock('./components/ResolutionPreviewCard', () => ({
  ResolutionPreviewCard: () => <div data-testid="resolution-preview-card" />,
}));

vi.mock('./components/ProviderProfilesList', () => ({
  ProviderProfilesList: () => <div data-testid="provider-profiles-list" />,
}));

vi.mock('./components/ProviderBindingsList', () => ({
  ProviderBindingsList: () => <div data-testid="provider-bindings-list" />,
}));

describe('ProvidersSection', () => {
  afterEach(() => {
    cleanup();
    refreshAll.mockClear();
  });

  test('renders split provider components and refreshes through hook', () => {
    render(<ProvidersSection />);

    expect(screen.getByRole('heading', { name: 'Providers' })).toBeInTheDocument();
    expect(screen.getByTestId('provider-profile-editor')).toBeInTheDocument();
    expect(screen.getByTestId('provider-bindings-card')).toBeInTheDocument();
    expect(screen.getByTestId('resolution-preview-card')).toBeInTheDocument();
    expect(screen.getByTestId('provider-profiles-list')).toBeInTheDocument();
    expect(screen.getByTestId('provider-bindings-list')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(refreshAll).toHaveBeenCalledTimes(1);
  });
});
