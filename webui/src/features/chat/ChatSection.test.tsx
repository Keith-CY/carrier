import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import { ChatSection } from './ChatSection';

vi.mock('./components/ChatMessages', () => ({
  ChatMessages: () => <div data-testid="chat-messages-block" />,
}));

vi.mock('./components/ChatComposer', () => ({
  ChatComposer: () => <div data-testid="chat-composer-block" />,
}));

describe('ChatSection', () => {
  afterEach(() => {
    cleanup();
  });

  test('renders split chat components', () => {
    render(<ChatSection data={{} as any} />);

    expect(screen.getByRole('heading', { name: 'Chat' })).toBeInTheDocument();
    expect(screen.getByTestId('chat-messages-block')).toBeInTheDocument();
    expect(screen.getByTestId('chat-composer-block')).toBeInTheDocument();
  });
});
