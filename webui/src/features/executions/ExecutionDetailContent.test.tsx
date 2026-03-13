import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import { ExecutionDetailContent } from './ExecutionDetailContent';

describe('ExecutionDetailContent', () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  test('renders detail blocks and downloads artifacts through callback', () => {
    const onDownloadArtifact = vi.fn();

    render(
      <ExecutionDetailContent
        execution={{
          id: 'exec-1',
          goal: 'Investigate weather',
          status: 'partial_completed',
          updatedAt: '2026-03-11T10:00:00Z',
          triggerSource: 'webhook',
          triggerId: 'trigger-1',
          triggerEvent: 'POST',
          initiator: 'tester',
          parentExecutionId: 'parent-1',
          sourceExecutionId: 'source-1',
          launchReason: 'rerun',
          requestedProvider: 'openrouter',
          memoryContractDigest: 'mem-digest',
          requiredMemory: ['shared:weather'],
          memoryProvenance: ['memory-1'],
          distillOutputs: ['distill-1'],
          authorization: {
            approvedBy: 'approver',
            approvedAt: '2026-03-11T10:01:00Z',
            infrastructureApproved: true,
          },
          policy: {
            decision: 'ask',
            requiresInfrastructureApproval: true,
            matchedRuleName: 'policy-1',
            reason: 'needs approval',
            toolPolicy: { mode: 'allowlist', allowedTools: ['weather'] },
            configuredMaxConcurrency: 2,
            effectiveMaxConcurrency: 1,
            maxTaskTimeoutMs: 5000,
            maxRetryBudget: 1,
            summary: 'policy summary',
            approvedBy: 'approver',
            approvedAt: '2026-03-11T10:02:00Z',
            targets: [{ hostId: 'host-1', agentId: 'zeroclaw', count: 1 }],
          },
          governance: {
            providerResolutions: [{
              hostId: 'host-1',
              agentId: 'zeroclaw',
              source: 'binding',
              provider: 'openrouter',
              model: 'gpt-4.1-mini',
              profileName: 'default',
              status: 'ok',
              syncMode: 'pull_validate_push',
              estimatedTotalTokens: 42,
              estimatedCostUsd: 0.12,
              successfulTasks: 1,
              failedTasks: 1,
              driftState: 'override',
              driftReason: 'instance override',
              avgLatencyMs: 230,
              trace: [{ source: 'instance', status: 'resolved', selected: true, provider: 'openrouter', model: 'gpt-4.1-mini' }],
              message: 'bound profile',
            }],
          },
          outcome: {
            summary: 'weather summarized',
            failureReason: 'tool timeout',
            failureCategory: 'timeout',
            artifacts: [{
              id: 'artifact-1',
              name: 'summary.txt',
              kind: 'text',
              contentType: 'text/plain',
              mediaType: 'audio/wav',
              source: 'telegram',
              externalId: 'tg-file-1',
              attachmentId: 'attachment-1',
              downloadUrl: '/downloads/artifact-1',
              sizeBytes: 12,
              createdAt: '2026-03-11T10:03:00Z',
            }],
          },
          taskUnits: [{ id: 'task-1', input: 'check weather' }],
          results: [{
            taskId: 'task-1',
            status: 'failed',
            summary: 'weather fetch failed',
            failureReason: 'tool timeout',
            failureCategory: 'timeout',
            output: 'partial output',
            hostId: 'host-1',
            agentId: 'zeroclaw',
            attempts: 2,
            latencyMs: 1234,
          }],
        }}
        workers={[{ hostId: 'host-1', agentId: 'zeroclaw', state: 'busy' }]}
        onDownloadArtifact={onDownloadArtifact}
      />,
    );

    expect(screen.getByRole('heading', { name: 'Investigate weather' })).toBeInTheDocument();
    expect(screen.getByText('Trigger')).toBeInTheDocument();
    expect(screen.getByText('Execution Lineage')).toBeInTheDocument();
    expect(screen.getByText('Outcome')).toBeInTheDocument();
    expect(screen.getByText('Execution Policy')).toBeInTheDocument();
    expect(screen.getByText('Approval & Governance')).toBeInTheDocument();
    expect(screen.getByText('Workers')).toBeInTheDocument();
    expect(screen.getByText('Task Results')).toBeInTheDocument();
    expect(screen.getByText(/media=audio\/wav/i)).toBeInTheDocument();
    expect(screen.getByText(/source=telegram/i)).toBeInTheDocument();
    expect(screen.getByText(/external=tg-file-1/i)).toBeInTheDocument();
    expect(screen.getByText(/attachment=attachment-1/i)).toBeInTheDocument();
    expect(screen.getByText(/\/downloads\/artifact-1/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('link', { name: 'Download summary.txt' }));
    expect(onDownloadArtifact).toHaveBeenCalledWith('artifact-1', 'summary.txt');
  });
});
