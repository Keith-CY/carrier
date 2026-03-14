import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import { PoliciesSection } from './PoliciesSection';

const refreshAll = vi.fn(async () => {});

vi.mock('./usePoliciesData', () => ({
  usePoliciesData: () => ({
    refreshAll,
    message: { type: 'success', text: 'ready' },
  }),
}));

vi.mock('./components/PolicyEditorCard', () => ({
  PolicyEditorCard: () => <div data-testid="policy-editor-card" />,
}));

vi.mock('./components/TriggerEditorCard', () => ({
  TriggerEditorCard: () => <div data-testid="trigger-editor-card" />,
}));

vi.mock('./components/PoliciesList', () => ({
  PoliciesList: () => <div data-testid="policies-list" />,
}));

vi.mock('./components/TriggersList', () => ({
  TriggersList: () => <div data-testid="triggers-list" />,
}));

describe('PoliciesSection', () => {
  afterEach(() => {
    cleanup();
    refreshAll.mockClear();
  });

  test('renders split policy components and refreshes through hook', () => {
    render(<PoliciesSection />);

    expect(screen.getByRole('heading', { name: 'Policies' })).toBeInTheDocument();
    expect(screen.getByTestId('policy-editor-card')).toBeInTheDocument();
    expect(screen.getByTestId('trigger-editor-card')).toBeInTheDocument();
    expect(screen.getByTestId('policies-list')).toBeInTheDocument();
    expect(screen.getByTestId('triggers-list')).toBeInTheDocument();
    expect(screen.getByText('ready')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(refreshAll).toHaveBeenCalledTimes(1);
  });
});
