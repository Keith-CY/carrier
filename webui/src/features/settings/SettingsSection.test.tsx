import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import { SettingsSection } from './SettingsSection';

vi.mock('./components/SettingsProviderCard', () => ({
  SettingsProviderCard: () => <div data-testid="settings-provider-card" />,
}));

vi.mock('./components/SettingsGatewayCard', () => ({
  SettingsGatewayCard: () => <div data-testid="settings-gateway-card" />,
}));

describe('SettingsSection', () => {
  afterEach(() => {
    cleanup();
  });

  test('renders split settings cards', () => {
    render(<SettingsSection data={{} as any} />);

    expect(screen.getByRole('heading', { name: 'Settings' })).toBeInTheDocument();
    expect(screen.getByTestId('settings-provider-card')).toBeInTheDocument();
    expect(screen.getByTestId('settings-gateway-card')).toBeInTheDocument();
  });
});
