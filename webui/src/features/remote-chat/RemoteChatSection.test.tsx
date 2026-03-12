import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import { RemoteChatSection } from './RemoteChatSection';

vi.mock('./components/RemoteChatToolbar', () => ({
  RemoteChatToolbar: () => <div data-testid="remote-chat-toolbar" />,
}));

vi.mock('./components/RemoteChatThread', () => ({
  RemoteChatThread: () => <div data-testid="remote-chat-thread" />,
}));

vi.mock('./components/RemoteChatCompose', () => ({
  RemoteChatCompose: () => <div data-testid="remote-chat-compose" />,
}));

describe('RemoteChatSection', () => {
  afterEach(() => {
    cleanup();
  });

  test('renders split remote chat components', () => {
    render(<RemoteChatSection data={{} as any} />);

    expect(screen.getByRole('heading', { name: 'Remote Chat' })).toBeInTheDocument();
    expect(screen.getByTestId('remote-chat-toolbar')).toBeInTheDocument();
    expect(screen.getByTestId('remote-chat-thread')).toBeInTheDocument();
    expect(screen.getByTestId('remote-chat-compose')).toBeInTheDocument();
  });
});
