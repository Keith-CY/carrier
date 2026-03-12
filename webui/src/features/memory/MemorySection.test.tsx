import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import { MemorySection } from './MemorySection';

vi.mock('./components/MemorySearchCard', () => ({
  MemorySearchCard: () => <div data-testid="memory-search-card" />,
}));

vi.mock('./components/MemoryActionsCard', () => ({
  MemoryActionsCard: () => <div data-testid="memory-actions-card" />,
}));

vi.mock('./components/MemoryPackagesCard', () => ({
  MemoryPackagesCard: () => <div data-testid="memory-packages-card" />,
}));

vi.mock('./components/MemoryResultsCard', () => ({
  MemoryResultsCard: () => <div data-testid="memory-results-card" />,
}));

describe('MemorySection', () => {
  afterEach(() => {
    cleanup();
  });

  test('renders split memory components', () => {
    render(
      <MemorySection
        data={{} as any}
      />,
    );

    expect(screen.getByTestId('memory-search-card')).toBeInTheDocument();
    expect(screen.getByTestId('memory-actions-card')).toBeInTheDocument();
    expect(screen.getByTestId('memory-packages-card')).toBeInTheDocument();
    expect(screen.getByTestId('memory-results-card')).toBeInTheDocument();
  });
});
