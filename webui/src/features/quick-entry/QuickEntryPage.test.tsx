import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import { QuickEntryPage } from './QuickEntryPage';

const approveExecution = vi.fn();
const cancelExecution = vi.fn();
const promptInstall = vi.fn();
const dismissInstall = vi.fn();

vi.mock('./useQuickEntryData', () => ({
  useQuickEntryData: () => ({
    approvals: [
      {
        id: 'exec-ask',
        goal: 'Approve production remediation',
        status: 'pending_authorization',
        project: 'checkout',
      },
    ],
    activeExecutions: [
      {
        id: 'exec-running',
        goal: 'Investigate checkout latency',
        status: 'running',
        project: 'carrier',
      },
    ],
    approvalCount: 1,
    activeCount: 1,
    latestMessage: { text: 'Continue the current incident thread.' },
    summarizeApproval: () => 'Needs explicit review.',
    summarizeActivity: () => 'Still running.',
    approveExecution,
    cancelExecution,
    approveMutation: { isPending: false },
    cancelMutation: { isPending: false },
    install: {
      canInstall: true,
      isStandalone: false,
      promptInstall,
      dismissInstall,
      installDismissed: false,
    },
    chat: {
      activeSessionId: 'session-1',
      recentProjects: [{ id: 'proj_alpha', name: 'Alpha' }],
      messages: [{ id: 'm-1', role: 'assistant', text: 'Continue the current incident thread.', createdAt: '2026-03-18T00:00:00Z' }],
      input: '',
      setInput: vi.fn(),
      statusText: 'Base agent ready.',
      send: vi.fn(),
      onKeyDown: vi.fn(),
      providerOverride: '',
      setProviderOverride: vi.fn(),
      selectedProjectId: '',
      setSelectedProjectId: vi.fn(),
      projectOptions: [],
      recentRuns: [],
      recentExecutions: [],
      runningAgents: 1,
      systemSummary: 'summary',
      starterPrompts: [],
      featureFlags: {},
      advancedOpen: false,
      setAdvancedOpen: vi.fn(),
      isStreaming: false,
      retryLast: vi.fn(),
      clearConversation: vi.fn(),
    },
  }),
}));

vi.mock('../chat/components/ChatMessages', () => ({
  ChatMessages: () => <div data-testid="quick-entry-chat-messages" />,
}));

vi.mock('../chat/components/ChatComposer', () => ({
  ChatComposer: () => <div data-testid="quick-entry-chat-composer" />,
}));

describe('QuickEntryPage', () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  test('renders approvals first and opens Home in a new tab', () => {
    render(<QuickEntryPage />);

    const waiting = screen.getByText('Waiting');
    const thread = screen.getByText('Thread');
    expect(waiting.compareDocumentPosition(thread) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    const homeLinks = screen.getAllByRole('link', { name: /home/i });
    expect(homeLinks[0]).toHaveAttribute('href', '/home');
    expect(homeLinks[0]).toHaveAttribute('target', '_blank');
  });

  test('wires install and execution actions', () => {
    render(<QuickEntryPage />);

    fireEvent.click(screen.getByRole('button', { name: /install carrier inbox/i }));
    fireEvent.click(screen.getByRole('button', { name: 'Approve' }));
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(promptInstall).toHaveBeenCalledTimes(1);
    expect(approveExecution).toHaveBeenCalledWith('exec-ask');
    expect(cancelExecution).toHaveBeenCalledWith('exec-ask');
  });
});
