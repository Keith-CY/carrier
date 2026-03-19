import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
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

  test('renders the chat-first home shell with starter actions', () => {
    render(
      <MemoryRouter>
        <ChatSection data={{} as any} />
      </MemoryRouter>,
    );

    expect(screen.getByRole('heading', { name: 'Home' })).toBeInTheDocument();
    expect(screen.getByLabelText(/Connect a provider and start from Home/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Start a task' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'What can you do?' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Show my active work' })).toBeInTheDocument();
    expect(screen.getByTestId('chat-messages-block')).toBeInTheDocument();
    expect(screen.getByTestId('chat-composer-block')).toBeInTheDocument();
  });
});
