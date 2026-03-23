package baseagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type subagentContextBroker struct {
	manager    *InMemorySubagentManager
	jobID      string
	contractID string
}

func (b *subagentContextBroker) Request(ctx context.Context, req DelegationContextRequest) (DelegationContextResponse, error) {
	if b == nil || b.manager == nil {
		return DelegationContextResponse{}, fmt.Errorf("subagent context broker is unavailable")
	}
	return b.manager.requestContext(ctx, b.jobID, b.contractID, req)
}

func (m *InMemorySubagentManager) requestContext(ctx context.Context, jobID, contractID string, req DelegationContextRequest) (DelegationContextResponse, error) {
	now := time.Now().UTC()
	normalized := normalizeDelegationContextRequest(req, contractID, now)
	if normalized.Question == "" {
		return DelegationContextResponse{}, fmt.Errorf("context request question is required")
	}

	m.mu.Lock()
	job, ok := m.jobs[jobID]
	if !ok {
		m.mu.Unlock()
		return DelegationContextResponse{}, fmt.Errorf("subagent job %s not found", jobID)
	}
	if normalized.RequestID == "" {
		normalized.RequestID = fmt.Sprintf("%s-ctx-%d", jobID, len(job.ContextRequests)+1)
	}
	job.ContextRequests = append(job.ContextRequests, normalized)
	job.Status = SubagentJobStatusAwaiting
	job.UpdatedAt = now
	m.jobs[jobID] = job
	waiterKey := m.contextWaiterKey(jobID, normalized.RequestID)
	waiter := make(chan DelegationContextResponse, 1)
	m.contextWaiters[waiterKey] = waiter
	m.persistLocked()
	m.mu.Unlock()

	select {
	case response := <-waiter:
		m.finalizeContextResponse(jobID, normalized.RequestID, response)
		return normalizeDelegationContextResponse(response, normalized.RequestID, time.Now().UTC()), nil
	case <-ctx.Done():
		status := DelegationContextStatusUnavailable
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = DelegationContextStatusTimedOut
		}
		response := normalizeDelegationContextResponse(DelegationContextResponse{
			Status:  status,
			Summary: strings.TrimSpace(ctx.Err().Error()),
		}, normalized.RequestID, time.Now().UTC())
		m.finalizeContextResponse(jobID, normalized.RequestID, response)
		if errors.Is(ctx.Err(), context.Canceled) && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return response, ctx.Err()
		}
		return response, nil
	}
}

func (m *InMemorySubagentManager) finalizeContextResponse(jobID, requestID string, response DelegationContextResponse) {
	if m == nil {
		return
	}
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()

	job, ok := m.jobs[jobID]
	if !ok {
		delete(m.contextWaiters, m.contextWaiterKey(jobID, requestID))
		return
	}
	delete(m.contextWaiters, m.contextWaiterKey(jobID, requestID))
	response = normalizeDelegationContextResponse(response, requestID, now)

	for idx := range job.ContextRequests {
		if strings.TrimSpace(job.ContextRequests[idx].RequestID) != requestID {
			continue
		}
		job.ContextRequests[idx].Status = response.Status
		break
	}

	replaced := false
	for idx := range job.ContextResponses {
		if strings.TrimSpace(job.ContextResponses[idx].RequestID) != requestID {
			continue
		}
		job.ContextResponses[idx] = response
		replaced = true
		break
	}
	if !replaced {
		job.ContextResponses = append(job.ContextResponses, response)
	}

	if !isTerminalSubagentJobStatus(job.Status) {
		if response.Status != DelegationContextStatusFulfilled {
			missing := requestID
			for _, item := range job.ContextRequests {
				if strings.TrimSpace(item.RequestID) == requestID && strings.TrimSpace(item.Question) != "" {
					missing = strings.TrimSpace(item.Question)
					break
				}
			}
			job.MissingContext = trimStringList(append(job.MissingContext, missing))
			job.Status = SubagentJobStatusDegraded
			job.Confidence = "low"
		} else {
			if job.Status == SubagentJobStatusAwaiting {
				job.Status = SubagentJobStatusRunning
			}
			if strings.TrimSpace(job.Confidence) == "" {
				job.Confidence = "medium"
			}
		}
	}

	job.UpdatedAt = now
	m.jobs[jobID] = job
	m.persistLocked()
}

func (m *InMemorySubagentManager) contextWaiterKey(jobID, requestID string) string {
	return strings.TrimSpace(jobID) + ":" + strings.TrimSpace(requestID)
}
